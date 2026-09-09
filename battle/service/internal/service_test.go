package internal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal"
	engine2 "github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf/battlecfg"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

func testInput(seed uint64) *pb.BattleInput {
	return &pb.BattleInput{ConfigVersion: "default", Seed: seed, Combatants: []*pb.BattleCombatantInput{
		{InstanceId: "p1", UnitConfigId: "hero", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Position: 0},
		{InstanceId: "e1", UnitConfigId: "enemy", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: 0},
	}}
}

func TestMemoryRepositoryClonesAndConfirms(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Now().UTC()
	report := internal.Report{BattleID: "b1", UID: 7, Result: resultStub("b1"), CreatedAt: now, UpdatedAt: now}
	if err := repo.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	report.Result.Events[0].Detail = "mutated"
	got, err := repo.Get(context.Background(), "b1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Events[0].Detail == "mutated" {
		t.Fatal("repository leaked a mutable reference")
	}
	if err := repo.Confirm(context.Background(), "b1"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(context.Background(), "b1")
	if !got.Confirmed {
		t.Fatal("report was not confirmed")
	}
	if _, err := repo.Get(context.Background(), "missing"); !errors.Is(err, internal.ErrReportNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func resultStub(id string) engine2.Result {
	return engine2.Result{BattleID: id, Events: []engine2.Event{{Sequence: 1, Detail: "original"}}, Replay: engine2.Replay{BattleID: id}}
}

func TestServiceLifecycleQueryRecentAndRebuild(t *testing.T) {
	configs, err := battlecfg.LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	reports := NewMemoryRepository()
	var mu sync.Mutex
	var notifications []proto.Message
	persistedBeforeFinished := false
	finished := make(chan *pb.BattleFinishedNtf, 4)
	notifier := internal.NotifyFunc(func(_ internal.Source, msg proto.Message) error {
		mu.Lock()
		notifications = append(notifications, msg)
		mu.Unlock()
		if done, ok := msg.(*pb.BattleFinishedNtf); ok {
			_, getErr := reports.Get(context.Background(), done.BattleId)
			persistedBeforeFinished = getErr == nil
			finished <- done
		}
		return nil
	})
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, reports, notifier, Options{RetryDelay: time.Hour}))
	t.Cleanup(tr.Shutdown)

	value, err := tr.Request(pid, &pb.StartBattleReq{UID: 9, ServerID: 3, SourceNodeId: 4, Input: testInput(11)}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	created := value.(*pb.BattleCreatedNtf)
	select {
	case done := <-finished:
		if done.BattleId != created.BattleId {
			t.Fatalf("finished id = %q, created = %q", done.BattleId, created.BattleId)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for battle")
	}
	if !persistedBeforeFinished {
		t.Fatal("Finished was emitted before report persistence")
	}

	mu.Lock()
	copy := append([]proto.Message(nil), notifications...)
	mu.Unlock()
	if len(copy) < 3 {
		t.Fatalf("notifications = %d, want Created + Action(s) + Finished", len(copy))
	}
	if _, ok := copy[0].(*pb.BattleCreatedNtf); !ok {
		t.Fatalf("first notification = %T, want BattleCreatedNtf", copy[0])
	}
	lastSequence := uint64(0)
	for _, msg := range copy[1 : len(copy)-1] {
		action, ok := msg.(*pb.BattleActionNtf)
		if !ok {
			t.Fatalf("middle notification = %T, want BattleActionNtf", msg)
		}
		if action.Event.Sequence <= lastSequence {
			t.Fatalf("non-increasing sequence %d after %d", action.Event.Sequence, lastSequence)
		}
		lastSequence = action.Event.Sequence
	}
	if _, ok := copy[len(copy)-1].(*pb.BattleFinishedNtf); !ok {
		t.Fatalf("last notification = %T, want BattleFinishedNtf", copy[len(copy)-1])
	}

	queryValue, err := tr.Request(pid, &pb.QueryBattleReq{BattleId: created.BattleId}).AwaitTimeout(time.Second)
	if err != nil || !queryValue.(*pb.QueryBattleAck).Found {
		t.Fatalf("query = %#v, %v", queryValue, err)
	}
	recentValue, err := tr.Request(pid, &QueryPlayerRecent{UID: 9}).AwaitTimeout(time.Second)
	if err != nil || len(recentValue.(*PlayerRecent).BattleIDs) != 1 {
		t.Fatalf("recent = %#v, %v", recentValue, err)
	}
	unknown, err := tr.Request(pid, &pb.RebuildBattleReq{BattleId: "unknown"}).AwaitTimeout(time.Second)
	if err != nil || unknown.(*pb.RebuildBattleAck).Accepted {
		t.Fatalf("unknown rebuild = %#v, %v", unknown, err)
	}
	mu.Lock()
	rebuildStart := len(notifications)
	mu.Unlock()
	known, err := tr.Request(pid, &pb.RebuildBattleReq{BattleId: created.BattleId}).AwaitTimeout(time.Second)
	if err != nil || !known.(*pb.RebuildBattleAck).Accepted {
		t.Fatalf("known rebuild = %#v, %v", known, err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuilt battle")
	}
	mu.Lock()
	defer mu.Unlock()
	if rebuildStart >= len(notifications) {
		t.Fatal("rebuild emitted no lifecycle notifications")
	}
	if _, ok := notifications[rebuildStart].(*pb.BattleCreatedNtf); !ok {
		t.Fatalf("first rebuild notification = %T, want BattleCreatedNtf", notifications[rebuildStart])
	}
}

type blockingRepository struct {
	*MemoryRepository
	started chan struct{}
	release chan struct{}
	first   atomic.Bool
}

// Save 只让第一次调用阻塞（等待 release），之后直接透传：用来卡住第一场
// 战斗、验证第二场战斗在独立 battle 上不受影响。
func (r *blockingRepository) Save(ctx context.Context, report internal.Report) error {
	if r.first.CompareAndSwap(false, true) {
		close(r.started)
		<-r.release
	}
	return r.MemoryRepository.Save(ctx, report)
}

// TestPerBattleActorsAreIsolated 证明每场战斗在独立 battle 内自洽：第一场在
// Save 阻塞时，第二场仍能正常落库并推送 Finished。
func TestPerBattleActorsAreIsolated(t *testing.T) {
	configs, err := battlecfg.LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	repo := &blockingRepository{MemoryRepository: NewMemoryRepository(), started: make(chan struct{}), release: make(chan struct{})}
	finished := make(chan uint64, 4)
	notifier := internal.NotifyFunc(func(_ internal.Source, msg proto.Message) error {
		if done, ok := msg.(*pb.BattleFinishedNtf); ok {
			finished <- done.Uid
		}
		return nil
	})
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, repo, notifier, Options{RetryDelay: time.Hour}))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(repo.release) }) }
	defer func() {
		release() // 释放阻塞的 Save，避免 Shutdown 等不到卡住的 battle
		tr.Shutdown()
	}()

	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 1, ServerID: 1, SourceNodeId: 1, Input: testInput(1)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("first battle Save did not start")
	}
	// 第二场战斗开始：即便第一场仍阻塞在 Save，第二场也应独立完成。
	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 2, ServerID: 1, SourceNodeId: 1, Input: testInput(2)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case uid := <-finished:
		if uid != 2 {
			t.Fatalf("second battle finished uid = %d, want 2 (first battle should still be blocked)", uid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second battle did not finish while first was blocked in Save")
	}
	// 释放第一场，它随后也应完成。
	release()
	select {
	case uid := <-finished:
		if uid != 1 {
			t.Fatalf("first battle finished uid = %d, want 1", uid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first battle did not finish after Save was released")
	}
}

func TestFinishedRetriesUntilAttemptLimit(t *testing.T) {
	configs, err := battlecfg.LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{}, 4)
	notifier := internal.NotifyFunc(func(_ internal.Source, msg proto.Message) error {
		if _, ok := msg.(*pb.BattleFinishedNtf); ok {
			finished <- struct{}{}
		}
		return nil
	})
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, NewMemoryRepository(), notifier, Options{RetryDelay: 10 * time.Millisecond, RetryAttempts: 3}))
	t.Cleanup(tr.Shutdown)
	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 8, ServerID: 1, SourceNodeId: 1, Input: testInput(88)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatalf("received only %d Finished attempts", i)
		}
	}
	select {
	case <-finished:
		t.Fatal("Finished exceeded configured retry attempt limit")
	case <-time.After(30 * time.Millisecond):
	}
}

// TestConfirmedReportRedeliversOnce 验证"库中已有已确认战报再次 Begin"：
// manager 不再自行组织战报投递，而是 spawn redeliver battle 补发一次
// Created/Action/Finished 后自停（结果已确认，不再重试，也不悬挂占用登记）。
func TestConfirmedReportRedeliversOnce(t *testing.T) {
	configs, err := battlecfg.LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	reports := NewMemoryRepository()
	finished := make(chan *pb.BattleFinishedNtf, 4)
	var created, actions atomic.Int64
	notifier := internal.NotifyFunc(func(_ internal.Source, msg proto.Message) error {
		switch m := msg.(type) {
		case *pb.BattleCreatedNtf:
			created.Add(1)
		case *pb.BattleActionNtf:
			actions.Add(1)
		case *pb.BattleFinishedNtf:
			finished <- m
		}
		return nil
	})
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, reports, notifier, Options{RetryDelay: 10 * time.Millisecond, RetryAttempts: 3}))
	t.Cleanup(tr.Shutdown)

	req := func(uid uint64, seed uint64) *pb.StartBattleReq {
		return &pb.StartBattleReq{UID: uid, ServerID: 1, SourceNodeId: 1, Input: testInput(seed)}
	}
	if _, err := tr.Request(pid, req(5, 7)).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	var first *pb.BattleFinishedNtf
	select {
	case first = <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("first battle did not finish")
	}
	// 由 manager 代转确认（旧客户端路径，fire-and-forget）：落到活跃 battle 完成落库后退出。
	if !tr.Send(pid, &pb.BattleResultConfirmedNtf{BattleId: first.BattleId, Uid: 5}) {
		t.Fatal("send confirm failed")
	}
	deadline := time.Now().Add(time.Second)
	for {
		report, getErr := reports.Get(context.Background(), first.BattleId)
		if getErr == nil && report.Confirmed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("report not confirmed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// 同一输入再次 Begin：应触发 redeliver battle 补发一次并自停，不再重试。
	createdBefore, finishedCount := created.Load(), 0
	if _, err := tr.Request(pid, req(5, 7)).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case done := <-finished:
		if done.BattleId != first.BattleId {
			t.Fatalf("redelivered battle id = %q, want %q", done.BattleId, first.BattleId)
		}
		finishedCount++
	case <-time.After(2 * time.Second):
		t.Fatal("confirmed report was not redelivered")
	}
	if created.Load() != createdBefore+1 {
		t.Fatalf("created notifications = %d, want one more (%d)", created.Load(), createdBefore)
	}
	if actions.Load() == 0 {
		t.Fatal("redelivery emitted no action notifications")
	}
	select {
	case <-finished:
		t.Fatal("confirmed redelivery retried Finished")
	case <-time.After(100 * time.Millisecond):
	}
	if finishedCount != 1 {
		t.Fatalf("redelivery finished count = %d", finishedCount)
	}
}

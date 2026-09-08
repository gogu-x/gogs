package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/engine"
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
	report := Report{BattleID: "b1", UID: 7, Result: resultStub("b1"), CreatedAt: now, UpdatedAt: now}
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
	if _, err := repo.Get(context.Background(), "missing"); !errors.Is(err, ErrReportNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func resultStub(id string) engine.Result {
	return engine.Result{BattleID: id, Events: []engine.Event{{Sequence: 1, Detail: "original"}}, Replay: engine.Replay{BattleID: id}}
}

func TestServiceLifecycleQueryRecentAndRebuild(t *testing.T) {
	configs, err := LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	reports := NewMemoryRepository()
	var mu sync.Mutex
	var notifications []proto.Message
	persistedBeforeFinished := false
	finished := make(chan *pb.BattleFinishedNtf, 4)
	notifier := NotifyFunc(func(_ Source, msg proto.Message) error {
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
	pid := tr.SpawnOne(New(configs, reports, notifier, Options{Workers: 1, QueueSize: 2, RetryDelay: time.Hour}))
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
	once    sync.Once
}

func (r *blockingRepository) Save(ctx context.Context, report Report) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return r.MemoryRepository.Save(ctx, report)
}

func TestServiceWorkQueueIsBounded(t *testing.T) {
	configs, err := LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	repo := &blockingRepository{MemoryRepository: NewMemoryRepository(), started: make(chan struct{}), release: make(chan struct{})}
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, repo, nil, Options{Workers: 1, QueueSize: 1, RetryDelay: time.Hour}))
	defer func() {
		close(repo.release)
		tr.Shutdown()
	}()

	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 1, ServerID: 1, SourceNodeId: 1, Input: testInput(1)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 2, ServerID: 1, SourceNodeId: 1, Input: testInput(2)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Request(pid, &pb.StartBattleReq{UID: 3, ServerID: 1, SourceNodeId: 1, Input: testInput(3)}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("third job should be rejected when worker and queue are occupied")
	}
}

func TestFinishedRetriesUntilAttemptLimit(t *testing.T) {
	configs, err := LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{}, 4)
	notifier := NotifyFunc(func(_ Source, msg proto.Message) error {
		if _, ok := msg.(*pb.BattleFinishedNtf); ok {
			finished <- struct{}{}
		}
		return nil
	})
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(configs, NewMemoryRepository(), notifier, Options{Workers: 1, QueueSize: 2, RetryDelay: 10 * time.Millisecond, RetryAttempts: 3}))
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

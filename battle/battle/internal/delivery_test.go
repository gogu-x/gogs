package internal

import (
	"context"
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// TestComputedReportRetriesAndRetires 验证结束通知按次数重试，并回收无人确认的 Actor。
func TestComputedReportRetriesAndRetires(t *testing.T) {
	if tlog.Log == nil {
		tlog.NewLog(t.TempDir(), 0)
	}
	setup := chargeTestSetup(chargeTestSkill())
	setup.Rules.TickDurationMS = 60000
	b, err := engine.NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	reports := NewMemoryRepository()
	finished := make(chan *pb.BattleFinishedNtf, 8)
	actor := NewBattleActor(Params{
		BattleID: setup.BattleID, UID: 7, Battle: b, Reports: reports,
		Source: Source{ServerID: 1, NodeID: 2}, RetryDelay: 20 * time.Millisecond, RetryAttempts: 2,
		Notifier: NotifyFunc(func(_ Source, message proto.Message) error {
			if result, ok := message.(*pb.BattleFinishedNtf); ok {
				finished <- result
			}
			return nil
		}),
	})
	pid := tree.SpawnOne(actor)
	t.Cleanup(func() { tree.Stop(pid) })
	var checksum string
	for i := 0; i < 2; i++ {
		select {
		case result := <-finished:
			if i == 0 {
				checksum = result.GetChecksum()
			} else if result.GetChecksum() != checksum {
				t.Fatal("重试重新计算了战斗结果")
			}
		case <-time.After(time.Second):
			t.Fatal("结束通知未按时重试")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, exists := tree.Lookup(def.BattleActorName(setup.BattleID)); !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("重试次数耗尽后没有回收 Actor")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-finished:
		t.Fatal("超过最大投递次数")
	default:
	}
	report, err := reports.Get(context.Background(), setup.BattleID)
	if err != nil || report.Confirmed || len(report.InitialUnits) != len(setup.Units) {
		t.Fatalf("未确认战报或初始快照丢失: report=%+v err=%v", report, err)
	}
}

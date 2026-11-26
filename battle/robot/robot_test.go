package robot

import (
	"fmt"
	"github.com/gogu-x/tree"
	"testing"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

// TestConfiguredRotation 验证机器人按配置轮换动作。
func TestConfiguredRotation(t *testing.T) {
	control := &pb.BattleControl{BasicSkill: "11"}
	sequence := []string{"3", "11", "4"}
	first, second := ConfiguredRotation(sequence), ConfiguredRotation(sequence)
	ok := &pb.BattleInputAck{Sequence: 1}
	rejected := &pb.BattleInputAck{Sequence: 2, Code: pb_common.ErrCode_INVALID_BATTLE_REQUEST}
	if first(control, nil) != "3" || first(control, ok) != "11" || first(control, ok) != "11" || first(control, rejected) != "11" || first(control, &pb.BattleInputAck{Sequence: 3}) != "4" || first(control, &pb.BattleInputAck{Sequence: 4}) != "3" || second(control, nil) != "3" {
		t.Fatal("循环顺序、回执重试或独立进度异常")
	}
}

// TestMultipleUnitsShareScheduler 验证多个单位共用调度器时仍能独立出手。
func TestMultipleUnitsShareScheduler(t *testing.T) {
	probe := &commandProbe{commands: make(chan *pb.BattleInputReq, 8)}
	target := tree.SpawnOne(probe)
	defer tree.Stop(target)
	const battleID = "multi-unit"
	actor := New(battleID, target, time.Now().Add(-time.Second))
	add := func(id string, pos int32, at int64) {
		control := &pb.BattleControl{UnitId: id}
		actor.AddUnit(control, pos, at, ConfiguredRotation([]string{id + "1", id + "2"}), nil)
	}
	add("slow", 1, 20)
	add("fast", 2, 10)
	add("other", 1, 10)
	pid := tree.SpawnOne(actor)
	defer tree.Stop(pid)
	want := func(id string, seq uint64) {
		t.Helper()
		select {
		case cmd := <-probe.commands:
			if cmd.GetUnitId() != id || cmd.GetSequence() != seq || cmd.GetSkillId() != fmt.Sprintf("%s%d", id, seq) {
				t.Fatalf("意外动作 %v", cmd)
			}
		case <-time.After(time.Second):
			t.Fatalf("未收到 %s 动作", id)
		}
	}
	want("other", 1)
	want("fast", 1)
	want("slow", 1)
	ack := func(id string) {
		tree.Send(pid, &pb.BattleInputAck{BattleId: battleID, UnitId: id, Sequence: 1, CurrentMs: 20, ReadyMs: 30})
	}
	tree.Send(pid, &UnitRemoved{UnitID: "other"})
	ack("other")
	ack("fast")
	ack("fast")
	want("fast", 2)
	ack("slow")
	want("slow", 2)
}

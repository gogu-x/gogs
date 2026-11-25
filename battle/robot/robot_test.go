package robot

import (
	"fmt"
	"github.com/gogu-x/tree"
	"testing"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

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

// commandProbe 接收实际调度器投递的动作，测试侧可独立控制各单位回执时机。
type commandProbe struct{ commands chan *pb.BattleInputReq }

func (*commandProbe) Name() string        { return "test-multi-unit-commands" }
func (*commandProbe) OnInit(tree.Context) {}
func (*commandProbe) OnStop(tree.Context) {}
func (p *commandProbe) HandleMessage(_ tree.Context, msg interface{}) {
	p.commands <- msg.(*Command).Input
}

func TestMultipleUnitsShareScheduler(t *testing.T) {
	probe := &commandProbe{commands: make(chan *pb.BattleInputReq, 8)}
	target := tree.SpawnOne(probe)
	defer tree.Stop(target)
	const battleID = "multi-unit"
	actor := New(battleID, target, time.Now().Add(-time.Second), time.Millisecond)
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
		tree.Send(pid, &pb.BattleInputAck{BattleId: battleID, UnitId: id, Sequence: 1, CurrentTick: 20, ReadyTick: 30})
	}
	tree.Send(pid, &UnitRemoved{UnitID: "other"})
	ack("other")
	ack("fast")
	ack("fast")
	want("fast", 2)
	ack("slow")
	want("slow", 2)
}

package engine

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// TestInteractiveCommands 验证即时战斗对动作指令的处理。
func TestInteractiveCommands(t *testing.T) {
	setup := testSetup()
	skill := setup.Units[0].BasicSkill
	skill.ID = "equipped-skill"
	skill.Cooldown = 3
	setup.Units[0].ActiveSkills = []SkillConfig{skill}
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	if due, ok := b.NextDueMS(); !ok || due != defaultDurationMS {
		t.Fatal("没有状态任务时应只等待战斗截止")
	}
	input := func(seq uint64, skill, target string) *pb.BattleInputAck {
		return b.ApplyInput(&pb.BattleInputReq{BattleId: b.ID(), UnitId: "a", Sequence: seq, SkillId: skill, TargetId: target}, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	}
	if input(2, "unknown", "d").GetCode() == pb_common.ErrCode_OK || input(3, "basic", "a").GetCode() == pb_common.ErrCode_OK {
		t.Fatal("非法动作被接受")
	}
	if b.ApplyInput(&pb.BattleInputReq{BattleId: b.ID(), UnitId: "d", Sequence: 1, SkillId: "basic"}, pb.BattleTeam_BATTLE_TEAM_ATTACKER).GetCode() == pb_common.ErrCode_OK {
		t.Fatal("越权控制防守方")
	}
	b.AdvanceClock(123)
	ack := input(4, skill.ID, "d")
	hp := b.UnitResults()[1].GetHp()
	if ack.GetCode() != pb_common.ErrCode_OK || hp >= 100 || ack.GetReadyMs() != 133 {
		t.Fatalf("技能没有即时生效 %v hp=%d", ack, hp)
	}
	if input(4, skill.ID, "d").GetAction() != ack.GetAction() || b.UnitResults()[1].GetHp() != hp {
		t.Fatal("重复结算")
	}
	b.AdvanceClock(132)
	if input(5, "basic", "d").GetCode() == pb_common.ErrCode_OK {
		t.Fatal("未到攻击间隔就再次出手")
	}
	b.AdvanceClock(133)
	if input(6, skill.ID, "d").GetCode() == pb_common.ErrCode_OK || input(7, "basic", "d").GetCode() != pb_common.ErrCode_OK {
		t.Fatal("技能冷却期间应可普攻")
	}
}

// TestStatusExpiresBeforeInputAtSameMillisecond 验证同一毫秒先移除到期状态，再结算玩家动作。
func TestStatusExpiresBeforeInputAtSameMillisecond(t *testing.T) {
	setup := testSetup()
	setup.Units[0].InitialStatus = []StatusConfig{{ID: "silence", Kind: pb.StatusType_STATUS_TYPE_SILENCE, DurationMS: 73}}
	active := setup.Units[0].BasicSkill
	active.ID = "spell"
	setup.Units[0].ActiveSkills = []SkillConfig{active}
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.BattleInputReq{BattleId: b.ID(), UnitId: "a", SkillId: active.ID, Sequence: 1}
	b.AdvanceClock(72)
	if len(b.StatusSnapshot()) != 1 {
		t.Fatal("沉默应持续到第 73 毫秒")
	}
	events := b.AdvanceClock(73)
	if len(events) == 0 || events[len(events)-1].GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED {
		t.Fatal("到期时应先产生移除事件")
	}
	ack := b.ApplyInput(req, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	if ack.GetCode() != pb_common.ErrCode_OK || ack.GetCurrentMs() != 73 || b.UnitResults()[1].GetHp() >= 100 {
		t.Fatalf("状态到期后技能应立即生效: %v", ack)
	}
}

// TestBattleDuration 验证连续出手和增援不会改变初始截止时间。
func TestBattleDuration(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxDurationMS = 1000
	setup.Units[1].MaxHP = 100000
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 30; i++ {
		b.AdvanceClock(int64(i * 10))
		ack := b.ApplyInput(&pb.BattleInputReq{BattleId: b.ID(), UnitId: "a", SkillId: "basic", Sequence: uint64(i)}, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
		if ack.GetCode() != pb_common.ErrCode_OK {
			t.Fatalf("第 %d 次出手失败", i)
		}
	}
	ally := setup.Units[0]
	ally.InstanceID = "ally"
	if err := b.AddAttacker(ally, true); err != nil {
		t.Fatal(err)
	}
	b.AdvanceClock(999)
	if b.InputDone() {
		t.Fatal("提前结束")
	}
	b.AdvanceClock(1200)
	result, err := b.Finalize()
	if err != nil || !b.InputDone() || result.ElapsedMs != 1000 || result.Outcome != pb.BattleOutcome_BATTLE_OUTCOME_DRAW {
		t.Fatalf("超时结算异常：%v %v", result.Outcome, err)
	}
}

package engine

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func TestInteractiveCommands(t *testing.T) {
	setup := testSetup()
	skill := setup.Units[0].BasicSkill
	skill.ID = "equipped-skill"
	skill.Cooldown = 3
	skill.WindupTicks = 20
	setup.Units[0].ActiveSkills = []SkillConfig{skill}
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.NextTick(); ok {
		t.Fatal("引擎不应主动安排攻击")
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
	ack := input(4, skill.ID, "d")
	hp := b.UnitResults()[1].GetHp()
	if ack.GetCode() != pb_common.ErrCode_OK || hp >= 100 || ack.GetReadyTick() != 10 {
		t.Fatalf("技能没有即时生效 %v hp=%d", ack, hp)
	}
	if input(4, skill.ID, "d").GetAction() != ack.GetAction() || b.UnitResults()[1].GetHp() != hp {
		t.Fatal("重复结算")
	}
	b.AdvanceClock(9)
	if input(5, "basic", "d").GetCode() == pb_common.ErrCode_OK {
		t.Fatal("未到攻击间隔就再次出手")
	}
	b.AdvanceClock(10)
	if input(6, skill.ID, "d").GetCode() == pb_common.ErrCode_OK || input(7, "basic", "d").GetCode() != pb_common.ErrCode_OK {
		t.Fatal("技能冷却期间应可普攻")
	}
}

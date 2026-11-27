package engine

import (
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"
)

// guardBattle 创建有两名玩家、一个会举刀的首领的确定性战斗。
func guardBattle(t *testing.T) *Battle {
	t.Helper()
	setup := testSetup()
	setup.Rules.ATBThreshold = 100000
	hero, boss := &setup.Units[0], &setup.Units[1]
	hero.GuardEnabled = true
	skill := hero.BasicSkill
	skill.ID = "spell"
	hero.ActiveSkills = []SkillConfig{skill}
	heavy := boss.BasicSkill
	heavy.ID = "heavy"
	heavy.WindupMS = 1500
	boss.ActiveSkills = []SkillConfig{heavy}
	ally := *hero
	ally.InstanceID = "ally"
	setup.Units = append(setup.Units, ally)
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// guardInput 按单位所属阵营提交测试指令。
func guardInput(
	b *Battle, unit, skill, target string, seq uint64,
) *pb.BattleInputAck {
	team := pb.BattleTeam_BATTLE_TEAM_ATTACKER
	if unit == "d" {
		team = pb.BattleTeam_BATTLE_TEAM_DEFENDER
	}
	return b.ApplyInput(&pb.BattleInputReq{BattleId: b.ID(), UnitId: unit, SkillId: skill, TargetId: target, Sequence: seq}, team)
}

// TestGuardCounter 验证预警、独立架势、格挡与一次性反击共用服务端时钟。
func TestGuardCounter(t *testing.T) {
	b := guardBattle(t)
	heavy := guardInput(b, "d", "heavy", "a", 1)
	initial := b.UnitResults()
	wind := initial[1].GetWindup()
	if heavy.GetCode() != common.ErrCode_OK || wind.GetAction() != heavy.GetAction() || wind.GetTargetId() != "a" || wind.GetImpactMs() != 1500 || initial[0].GetHp() != 100 {
		t.Fatalf("举刀时应锁敌并保留血量: %v %v", heavy, initial)
	}
	if due, _ := b.NextDueMS(); due != 1500 {
		t.Fatal("时间线缺少待命中重击", due)
	}
	if ack := guardInput(b, "d", "basic", "a", 2); ack.GetCode() == common.ErrCode_OK {
		t.Fatal("举刀期间不能再出招")
	}
	b.AdvanceClock(800)
	if ack := guardInput(b, "a", "basic", "d", 1); ack.GetReadyMs() != 1800 {
		t.Fatal(ack)
	}
	b.AdvanceClock(850)
	guard := guardInput(b, "a", "guard", "", 2)
	if guard.GetCode() != common.ErrCode_OK || guard.GetReadyMs() != 1850 || guardInput(b, "a", "guard", "", 2) != guard {
		t.Fatal("普攻冷却中应可进入架势且不得重复入账", guard)
	}
	if ack := guardInput(b, "a", "guard", "", 3); ack.GetCode() == common.ErrCode_OK {
		t.Fatal("架势自身冷却应限制重复使用")
	}
	if control := b.Controls()[0]; !control.GetCanGuard() || control.GetGuardReadyMs() != 3850 {
		t.Fatal(control)
	}
	b.Flush()
	if events := b.AdvanceClock(1499); len(events) != 0 {
		t.Fatal(events)
	}
	events := b.AdvanceClock(1500)
	if len(events) != 2 || !events[0].GetGuarded() || events[0].GetCounter() || events[0].GetAmount() != 3 || !events[0].GetUpdatedUnit().GetCounterReady() ||
		events[1].GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED || events[0].GetAction() != heavy.GetAction() || events[1].GetAction() != heavy.GetAction() {
		t.Fatalf("命中应广播同一动作的减伤和反击就绪: %v", events)
	}
	units := b.UnitResults()
	if units[0].GetHp() != 97 || units[0].GetGuardUntilMs() != 0 || !units[0].GetCounterReady() || units[1].GetWindup() != nil || !initial[0].GetAlive() || initial[1].GetWindup() == nil {
		t.Fatal("状态或初始快照被覆盖", units)
	}
	b.AdvanceClock(1850)
	if ack := guardInput(b, "a", "spell", "d", 4); ack.GetCode() != common.ErrCode_OK || !b.UnitResults()[0].GetCounterReady() {
		t.Fatal("主动技能不消耗反击", ack)
	}
	b.Flush()
	b.AdvanceClock(2850)
	ack := guardInput(b, "a", "basic", "d", 5)
	events = b.Flush()
	if ack.GetCode() != common.ErrCode_OK || len(events) != 4 || !events[2].GetCounter() || events[2].GetAmount() != 30 || events[2].GetUpdatedUnit().GetCounterReady() {
		t.Fatal("下一刀应消耗反击并只结算一次", events)
	}
	if guardInput(b, "a", "basic", "d", 5) != ack || len(b.Flush()) != 0 {
		t.Fatal("重投普攻重复反击")
	}
	b.AdvanceClock(3850)
	guardInput(b, "a", "basic", "d", 6)
	if hit := b.Flush()[2]; hit.GetCounter() || hit.GetAmount() != 20 {
		t.Fatal("反击影响了第二刀", hit)
	}
}

// TestGuardTimingAndOwnership 验证截止毫秒、锁定目标及防守单位权限。
func TestGuardTimingAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, unit string
		at         int64
	}{
		{"到期毫秒", "a", 700}, {"命中毫秒", "a", 1500}, {"队友不能代挡", "ally", 850},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBattle(t)
			if guardInput(b, "d", "guard", "", 1).GetCode() == common.ErrCode_OK {
				t.Fatal("机器人不应获得玩家架势")
			}
			guardInput(b, "d", "heavy", "a", 2)
			b.AdvanceClock(tc.at)
			if guardInput(b, tc.unit, "guard", "", 1).GetCode() != common.ErrCode_OK {
				t.Fatal("架势提交失败")
			}
			b.AdvanceClock(1500)
			if u := b.UnitResults()[0]; u.GetHp() != 90 || u.GetCounterReady() {
				t.Fatal("过期或其他单位的架势影响伤害", u)
			}
		})
	}
}

// TestWindupTargetDeath 验证队友仍在时，重击不会改攻已经锁定目标以外的角色。
func TestWindupTargetDeath(t *testing.T) {
	b := guardBattle(t)
	guardInput(b, "d", "heavy", "a", 1)
	b.inputs["a"].actor.HP = 0
	b.Flush()
	events := b.AdvanceClock(1500)
	if len(events) != 2 || events[0].GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED || events[0].GetTargetId() != "a" || b.UnitResults()[2].GetHp() != 100 {
		t.Fatal("目标退场应取消重击", events)
	}
}

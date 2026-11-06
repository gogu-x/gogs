package internal

import (
	"reflect"
	"testing"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func timedStatusSetup() engine.Setup {
	setup := chargeTestSetup(chargeTestSkill())
	setup.Rules.TickDurationMS = 100
	setup.Rules.MaxActions = 20
	for index := range setup.Units {
		setup.Units[index].MaxHP = 10000
	}
	return setup
}

func TestTimedPeriodicStatusUsesLogicalTime(t *testing.T) {
	for _, kind := range []pb.StatusType{pb.StatusType_STATUS_TYPE_DOT, pb.StatusType_STATUS_TYPE_HOT} {
		setup := timedStatusSetup()
		setup.Units[0].InitialStatus = []engine.StatusConfig{{ID: "periodic", Kind: kind, DurationMS: 3000, IntervalMS: 1000, Potency: 7}}
		result := runChargeTest(t, setup)
		var ticks []int64
		removed := 0
		for _, event := range result.Events {
			if event.GetStatusId() != "periodic" {
				continue
			}
			switch event.GetType() {
			case pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL:
				ticks = append(ticks, event.GetTick())
				if event.GetStatusInstanceId() == 0 || event.GetActorId() != "attacker" {
					t.Fatal("周期事件缺少来源或实例")
				}
			case pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED:
				removed++
				if event.GetTick() != 30 {
					t.Fatal("状态没有在三秒到期")
				}
			}
		}
		if !reflect.DeepEqual(ticks, []int64{10, 20}) || removed != 1 {
			t.Fatalf("周期结算=%v，移除次数=%d", ticks, removed)
		}
		battle, err := engine.NewBattle(setup)
		if err != nil {
			t.Fatal(err)
		}
		for {
			if _, ok := battle.NextTick(); !ok {
				break
			}
			battle.Advance()
		}
		incremental, err := battle.Finalize()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result, incremental) {
			t.Fatal("限时状态的整场计算与分步推进不一致")
		}
	}
}

func TestTimedStatusExpiresBeforeSameTickImpact(t *testing.T) {
	setup := timedStatusSetup()
	setup.Rules.MaxActions = 1
	setup.Units[0].InitialStatus = []engine.StatusConfig{{ID: "attack", DurationMS: 1100, Modifier: engine.AttributeModifier{AttackFlat: 100}}}
	result := runChargeTest(t, setup)
	removed := false
	for _, event := range result.Events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED {
			removed = true
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			if !removed || event.GetTick() != 11 || event.GetAmount() != 30 {
				t.Fatalf("到期应先于同帧伤害: %+v", event)
			}
			return
		}
	}
	t.Fatal("缺少伤害事件")
}

func TestSkillAppliesTimedStatusAndBattleDoesNotWaitForExpiry(t *testing.T) {
	setup := timedStatusSetup()
	setup.Rules.MaxActions = 1
	status := engine.StatusConfig{ID: "debuff", DurationMS: 300000}
	setup.Units[0].BasicSkill.Effects = []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS, Status: &status}}
	result := runChargeTest(t, setup)
	applied := false
	for _, event := range result.Events {
		if event.GetTick() >= 3000 {
			t.Fatal("战斗不应等待状态到期才结束")
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED {
			if event.GetTick() != 11 || event.GetStatusExpireTick() != 3011 || event.GetStatusInstanceId() == 0 || event.GetActorId() != "attacker" || event.GetTargetId() != "enemy-1" {
				t.Fatalf("动态状态元数据错误: %+v", event)
			}
			applied = true
		}
	}
	if !applied {
		t.Fatal("技能没有施加限时状态")
	}
}

func TestTurnStatusKeepsActionBasedLifetime(t *testing.T) {
	setup := timedStatusSetup()
	setup.Units[0].InitialStatus = []engine.StatusConfig{{ID: "turns", Kind: pb.StatusType_STATUS_TYPE_DOT, DurationTurns: 2, Potency: 7}}
	result := runChargeTest(t, setup)
	var ticks []int64
	removed := 0
	for _, event := range result.Events {
		if event.GetStatusId() != "turns" {
			continue
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			ticks = append(ticks, event.GetTick())
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED {
			removed++
			if event.GetTick() != 22 || event.GetStatusExpireTick() != 0 {
				t.Fatal("回合状态应在第二次行动结束时移除")
			}
		}
	}
	if !reflect.DeepEqual(ticks, []int64{10, 20}) || removed != 1 {
		t.Fatalf("回合结算=%v，移除次数=%d", ticks, removed)
	}
}

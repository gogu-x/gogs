package internal

import (
	"testing"

	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func chargeTestSkill(charges ...engine.ChargeEffectConfig) engine.SkillConfig {
	return engine.SkillConfig{
		ID: "attack", TargetRule: pb.TargetRule_TARGET_RULE_ENEMY_SINGLE,
		Effects: []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_DAMAGE, CoefficientPermille: 1000}},
		Charges: charges, WindupTicks: 1, RecoveryTicks: 1,
	}
}

func chargeTestSetup(skill engine.SkillConfig) engine.Setup {
	return engine.Setup{
		BattleID: "charge-test", Seed: 17,
		Rules: engine.Rules{ATBThreshold: 1000, MaxActions: 2, CritMultiplierPermille: 1000},
		Units: []engine.UnitConfig{
			{InstanceID: "attacker", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Position: 1, MaxHP: 200, Attack: 30, Speed: 100, BasicSkill: skill},
			{InstanceID: "enemy-1", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: 1, MaxHP: 20, Attack: 1, Speed: 1, BasicSkill: skill},
			{InstanceID: "enemy-2", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: 2, MaxHP: 100, Attack: 1, Speed: 1, BasicSkill: skill},
			{InstanceID: "enemy-3", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: 3, MaxHP: 100, Attack: 1, Speed: 1, BasicSkill: skill},
		},
		Strategies: configcompiler.DefaultStrategies(),
	}
}

func runChargeTest(t *testing.T, setup engine.Setup) engine.Result {
	t.Helper()
	battle, err := engine.NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func burstTargets(result engine.Result, kind pb.EffectType) []string {
	ids := make([]string, 0)
	for _, event := range result.Events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetEffectType() == kind {
			ids = append(ids, event.GetTargetId())
		}
	}
	return ids
}

func TestChargeSharedAcrossTargetsAndBurstHitsLivingEnemies(t *testing.T) {
	skill := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 2, Gain: 1, Trigger: true,
		TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1000},
	})
	result := runChargeTest(t, chargeTestSetup(skill))
	targets := burstTargets(result, pb.EffectType_EFFECT_TYPE_ICE_BURST)
	if len(targets) != 2 || targets[0] != "enemy-2" || targets[1] != "enemy-3" {
		t.Fatalf("冰爆目标 = %v, 期望两个存活敌人", targets)
	}
	for _, event := range result.Events {
		if event.GetEffectType() == pb.EffectType_EFFECT_TYPE_ICE_BURST && event.GetAction() != 2 {
			t.Fatalf("冰爆提前触发: %#v", event)
		}
	}
}

func TestIceBurstCriticalUsesProtocolFields(t *testing.T) {
	skill := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 1, Gain: 1, Trigger: true,
		TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1000},
	})
	setup := chargeTestSetup(skill)
	setup.Rules.MaxActions = 1
	setup.Rules.CritChancePermille = 1000
	setup.Rules.CritMultiplierPermille = 2000
	for index := 1; index < len(setup.Units); index++ {
		setup.Units[index].MaxHP = 200
	}
	result := runChargeTest(t, setup)
	criticalBursts := 0
	for _, event := range result.Events {
		if event.GetEffectType() != pb.EffectType_EFFECT_TYPE_ICE_BURST {
			continue
		}
		if !event.GetCritical() {
			t.Fatalf("冰爆暴击未写入协议字段: %#v", event)
		}
		criticalBursts++
	}
	if criticalBursts != 3 {
		t.Fatalf("冰爆暴击事件数量 = %d，期望 3", criticalBursts)
	}
}

func TestPeriodicDamageUsesProtocolStatusType(t *testing.T) {
	setup := chargeTestSetup(chargeTestSkill())
	setup.Rules.MaxActions = 1
	setup.Units[0].InitialStatus = []engine.StatusConfig{{
		ID: "burn", Kind: pb.StatusType_STATUS_TYPE_DOT, DurationTurns: 2, Potency: 3,
	}}
	result := runChargeTest(t, setup)
	for _, event := range result.Events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetStatusId() == "burn" {
			if event.GetStatusType() != pb.StatusType_STATUS_TYPE_DOT {
				t.Fatalf("持续伤害未写入状态类型: %#v", event)
			}
			return
		}
	}
	t.Fatal("未找到持续伤害事件")
}

func TestChargeIsIndependentForEachActor(t *testing.T) {
	skill := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 2, Gain: 1, Trigger: true,
		TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1000},
	})
	setup := chargeTestSetup(skill)
	setup.Units[1].MaxHP = 100
	setup.Units = append(setup.Units, engine.UnitConfig{
		InstanceID: "attacker-2", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Position: 2,
		MaxHP: 200, Attack: 30, Speed: 90, BasicSkill: skill,
	})
	result := runChargeTest(t, setup)
	if targets := burstTargets(result, pb.EffectType_EFFECT_TYPE_ICE_BURST); len(targets) != 0 {
		t.Fatalf("两个角色各命中一次不应合并积攒值，冰爆目标 = %v", targets)
	}
}

func TestSoulJadePassiveChargeIsIndependentForEachActor(t *testing.T) {
	skill := chargeTestSkill()
	charge := engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 2, Gain: 1, Trigger: true,
		TargetRule:    pb.TargetRule_TARGET_RULE_ENEMY_ALL,
		TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1500},
	}
	setup := chargeTestSetup(skill)
	setup.Units[0].PassiveCharges = []engine.ChargeEffectConfig{charge}
	setup.Units = append(setup.Units, engine.UnitConfig{
		InstanceID: "attacker-2", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Position: 2,
		MaxHP: 200, Attack: 30, Speed: 90, BasicSkill: skill,
		PassiveCharges: []engine.ChargeEffectConfig{charge},
	})
	result := runChargeTest(t, setup)
	if targets := burstTargets(result, pb.EffectType_EFFECT_TYPE_ICE_BURST); len(targets) != 0 {
		t.Fatalf("两个角色的魂玉积攒值不应合并，冰爆目标 = %v", targets)
	}
}

func TestIceAndThunderChargeIndependently(t *testing.T) {
	skill := chargeTestSkill(
		engine.ChargeEffectConfig{
			Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 2, Gain: 1, Trigger: true,
			TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1000},
		},
		engine.ChargeEffectConfig{
			Kind: pb.EffectType_EFFECT_TYPE_THUNDER, Threshold: 2, Gain: 1, Trigger: true,
			TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_THUNDER, CoefficientPermille: 1000},
		},
	)
	skill.TargetRule = pb.TargetRule_TARGET_RULE_ENEMY_ALL
	setup := chargeTestSetup(skill)
	setup.Rules.MaxActions = 1
	setup.Units[1].MaxHP = 200
	setup.Units[2].MaxHP = 200
	setup.Units[3].MaxHP = 200
	result := runChargeTest(t, setup)
	for _, kind := range []pb.EffectType{pb.EffectType_EFFECT_TYPE_ICE_BURST, pb.EffectType_EFFECT_TYPE_THUNDER} {
		if targets := burstTargets(result, kind); len(targets) != 3 {
			t.Fatalf("%s 应在群攻命中后触发一次并攻击三个敌人，实际目标 = %v", kind, targets)
		}
	}
	lastBase, firstIce, firstThunder := uint64(0), uint64(0), uint64(0)
	for _, event := range result.Events {
		if event.GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			continue
		}
		switch event.GetEffectType() {
		case pb.EffectType_EFFECT_TYPE_DAMAGE:
			lastBase = event.GetSequence()
		case pb.EffectType_EFFECT_TYPE_ICE_BURST:
			if firstIce == 0 {
				firstIce = event.GetSequence()
			}
		case pb.EffectType_EFFECT_TYPE_THUNDER:
			if firstThunder == 0 {
				firstThunder = event.GetSequence()
			}
		}
	}
	if lastBase == 0 || firstIce <= lastBase || firstThunder <= firstIce {
		t.Fatalf("原技能、冰爆和天雷的事件顺序错误: base=%d ice=%d thunder=%d", lastBase, firstIce, firstThunder)
	}
}

func TestChargeCanWaitForTriggerSkill(t *testing.T) {
	build := chargeTestSkill(engine.ChargeEffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 1, Gain: 1})
	build.ID = "build"
	build.Cooldown = 2
	trigger := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 1, Trigger: true,
		TriggerEffect: engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, CoefficientPermille: 1000},
	})
	setup := chargeTestSetup(trigger)
	setup.Units[0].ActiveSkills = []engine.SkillConfig{build}
	setup.Units[1].MaxHP = 100
	result := runChargeTest(t, setup)
	for _, event := range result.Events {
		if event.GetEffectType() == pb.EffectType_EFFECT_TYPE_ICE_BURST && (event.GetAction() != 2 || event.GetSkillId() != "attack") {
			t.Fatalf("冰爆应由第二次行动的引爆技能触发: %#v", event)
		}
	}
	if targets := burstTargets(result, pb.EffectType_EFFECT_TYPE_ICE_BURST); len(targets) != 3 {
		t.Fatalf("引爆技能未攻击敌方全体，目标 = %v", targets)
	}
}

func TestChargeThresholdMustAgreeAcrossOneActorsSkills(t *testing.T) {
	skill := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 2, Gain: 1,
	})
	setup := chargeTestSetup(skill)
	active := chargeTestSkill(engine.ChargeEffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Threshold: 3, Gain: 1,
	})
	active.ID = "active"
	setup.Units[0].ActiveSkills = []engine.SkillConfig{active}
	if _, err := engine.NewBattle(setup); err == nil {
		t.Fatal("同一角色的冰积攒阈值不一致时应拒绝创建战斗")
	}
}

package internal

import (
	"reflect"
	"testing"

	"github.com/gogu-x/gogs/battle/battle/internal/battleconfig"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func supportTestSetup() engine.Setup {
	basic := chargeTestSkill()
	basic.WindupTicks, basic.RecoveryTicks = 3, 3
	offense := basic
	offense.ID, offense.Cooldown = "offense", 2
	return engine.Setup{
		BattleID: "independent-support", Seed: 17,
		Rules: engine.Rules{ATBThreshold: 1000, MaxActions: 7, TickDurationMS: 100, CritMultiplierPermille: 1000},
		Units: []engine.UnitConfig{
			{InstanceID: "attacker", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, MaxHP: 10000, Attack: 30, Speed: 200,
				BasicSkill: basic, ActiveSkills: []engine.SkillConfig{offense},
				InitialStatus: []engine.StatusConfig{{ID: "turn-dot", Kind: pb.StatusType_STATUS_TYPE_DOT, DurationTurns: 7, Potency: 60}}},
			{InstanceID: "enemy", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, MaxHP: 10000, Attack: 1, Speed: 1, BasicSkill: basic},
		},
		Strategies: battleconfig.DefaultStrategies(),
	}
}

func supportHealSkill() engine.SkillConfig {
	return engine.SkillConfig{
		ID: "heal", Cooldown: 3, TargetRule: pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP,
		Effects:     []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_HEAL, Flat: 20}},
		WindupTicks: 40, RecoveryTicks: 30,
	}
}

func supportBuffSkill() engine.SkillConfig {
	return engine.SkillConfig{
		ID: "buff", Cooldown: 2, TargetRule: pb.TargetRule_TARGET_RULE_SELF,
		Effects: []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS,
			Status: &engine.StatusConfig{ID: "defense", Kind: pb.StatusType_STATUS_TYPE_ATTRIBUTE_MODIFIER,
				DurationTurns: 2, Modifier: engine.AttributeModifier{DefenseFlat: 2}}}},
		WindupTicks: 40, RecoveryTicks: 30,
	}
}

func supportSkillUses(result engine.Result, skill string) []*pb.BattleEvent {
	var events []*pb.BattleEvent
	for _, event := range result.GetEvents() {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED && event.GetSkillId() == skill {
			events = append(events, event)
		}
	}
	return events
}

func supportSkillTicks(result engine.Result, skill string) []int64 {
	var ticks []int64
	for _, event := range supportSkillUses(result, skill) {
		ticks = append(ticks, event.GetTick())
	}
	return ticks
}

// TestSupportPreservesAttacksAndOwnCooldowns 用真实效果策略验证治疗、增益和攻击同时生效。
func TestSupportPreservesAttacksAndOwnCooldowns(t *testing.T) {
	setup := supportTestSetup()
	baseline := runChargeTest(t, setup)
	setup.Units[0].ActiveSkills = []engine.SkillConfig{setup.Units[0].ActiveSkills[0], supportHealSkill(), supportBuffSkill()}
	result := runChargeTest(t, setup)
	for _, skill := range []string{"offense", "attack"} {
		before, after := supportSkillUses(baseline, skill), supportSkillUses(result, skill)
		if len(before) != len(after) {
			t.Fatalf("辅助技能挤占 %s: %d -> %d", skill, len(before), len(after))
		}
		for index, event := range before {
			actual := after[index]
			if event.GetStartTick() != actual.GetStartTick() || event.GetImpactTick() != actual.GetImpactTick() || event.GetEndTick() != actual.GetEndTick() {
				t.Fatalf("辅助技能改变攻击节拍: %v -> %v", event, actual)
			}
		}
	}
	for skill, expected := range map[string][]int64{"heal": {5, 20, 35}, "buff": {5, 15, 25, 35}} {
		if actual := supportSkillTicks(result, skill); !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%s 冷却触发帧 = %v, 期望 %v", skill, actual, expected)
		}
		for _, event := range supportSkillUses(result, skill) {
			if event.GetStartTick() != event.GetTick() || event.GetImpactTick() != event.GetTick() || event.GetEndTick() != event.GetTick() || event.GetBasicAttack() {
				t.Fatalf("辅助技能仍占用动作窗口: %v", event)
			}
		}
	}
	heals, hits, periodic := 0, 0, 0
	var removed []int64
	for _, event := range result.GetEvents() {
		switch event.GetType() {
		case pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL:
			if event.GetAmount() != 20 || event.GetHpAfter()-event.GetHpBefore() != 20 || event.GetTargetId() != "attacker" {
				t.Fatalf("治疗没有真实生效: %v", event)
			}
			heals++
		case pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE:
			if event.GetStatusId() == "turn-dot" {
				periodic++
			} else {
				hits++
			}
		case pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED:
			if event.GetStatusId() == "defense" {
				removed = append(removed, event.GetTick())
			}
		}
	}
	if heals != 3 || hits != 7 || periodic != 7 || !reflect.DeepEqual(removed, []int64{15, 25, 35}) {
		t.Fatalf("附加效果不能重复推进状态/行动上限: heals=%d hits=%d periodic=%d removed=%v", heals, hits, periodic, removed)
	}
	verifySupportEventWindows(t, result)
	t.Logf("攻击保持每 5 tick 一次，共 %d 次；回血 %v；Buff %v", hits, supportSkillTicks(result, "heal"), supportSkillTicks(result, "buff"))
}

func verifySupportEventWindows(t *testing.T, result engine.Result) {
	t.Helper()
	uses := make(map[uint64]*pb.BattleEvent)
	ends := make(map[uint64]*pb.BattleEvent)
	for index, event := range result.GetEvents() {
		if event.GetSequence() != uint64(index+1) {
			t.Fatal("事件序号不连续")
		}
		switch event.GetType() {
		case pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED:
			if uses[event.GetAction()] != nil {
				t.Fatal("辅助效果复用了攻击 action 编号")
			}
			uses[event.GetAction()] = event
		case pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED:
			ends[event.GetAction()] = event
		case pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL,
			pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE,
			pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED:
			if event.GetStatusInstanceId() != 0 && event.GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED {
				continue
			}
			if event.GetSkillId() == "" {
				continue
			}
			use := uses[event.GetAction()]
			if use == nil || use.GetSkillId() != event.GetSkillId() || use.GetImpactTick() != event.GetTick() {
				t.Fatalf("生效事件与技能窗口不对应: %v", event)
			}
		}
	}
	for id, use := range uses {
		if end := ends[id]; end == nil || end.GetTick() != use.GetEndTick() {
			t.Fatalf("action %d 缺少一致的结束事件", id)
		}
	}
}

// TestSupportHonorsControlAndDeath 保留沉默、眩晕和回合持续伤害致死的限制。
func TestSupportHonorsControlAndDeath(t *testing.T) {
	for _, kind := range []pb.StatusType{pb.StatusType_STATUS_TYPE_SILENCE, pb.StatusType_STATUS_TYPE_STUN, pb.StatusType_STATUS_TYPE_DOT} {
		t.Run(kind.String(), func(t *testing.T) {
			setup := supportTestSetup()
			setup.Rules.MaxActions = 1
			setup.Units[0].InitialStatus = []engine.StatusConfig{{ID: "blocked", Kind: kind, DurationTurns: 2, Potency: 10000}}
			setup.Units[0].ActiveSkills = []engine.SkillConfig{supportHealSkill(), supportBuffSkill()}
			result := runChargeTest(t, setup)
			if len(supportSkillUses(result, "heal")) != 0 || len(supportSkillUses(result, "buff")) != 0 {
				t.Fatal("受控或已死亡角色仍触发了辅助技能")
			}
			wantAttacks := 0
			if kind == pb.StatusType_STATUS_TYPE_SILENCE {
				wantAttacks = 1
			}
			if len(supportSkillUses(result, "attack")) != wantAttacks {
				t.Fatal("沉默应保留普攻，眩晕和死亡不应攻击")
			}
		})
	}
}

// TestZeroCooldownSupportIsBoundedAndDeterministic 防止辅助触发反复扣冷却或生成无限同帧行动。
func TestZeroCooldownSupportIsBoundedAndDeterministic(t *testing.T) {
	setup := supportTestSetup()
	heal, buff := supportHealSkill(), supportBuffSkill()
	heal.Cooldown, buff.Cooldown = 0, 0
	setup.Units[0].ActiveSkills = []engine.SkillConfig{heal, buff}
	result := runChargeTest(t, setup)
	for _, skill := range []string{"attack", "heal", "buff"} {
		if actual := supportSkillTicks(result, skill); !reflect.DeepEqual(actual, []int64{5, 10, 15, 20, 25, 30, 35}) {
			t.Fatalf("零冷却重复触发或占用攻击: %s %v", skill, actual)
		}
	}
	verifySupportEventWindows(t, result)
	battle, err := engine.NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	var streamed []*pb.BattleEvent
	for {
		if _, ok := battle.NextTick(); !ok {
			break
		}
		streamed = append(streamed, battle.Advance()...)
	}
	incremental, err := battle.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	streamed = append(streamed, battle.Flush()...)
	if !reflect.DeepEqual(result, incremental) || !reflect.DeepEqual(result.GetEvents(), streamed) {
		t.Fatal("附加技能导致完整计算与按帧推进结果不一致")
	}
}

// TestSupportHotUsesStatusClock 持续治疗使用自身状态时钟，不因附加施放额外跳数。
func TestSupportHotUsesStatusClock(t *testing.T) {
	setup := supportTestSetup()
	heal := supportHealSkill()
	heal.Cooldown = 20
	heal.Effects = append(heal.Effects, engine.EffectConfig{
		Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS,
		Status: &engine.StatusConfig{ID: "regen", Kind: pb.StatusType_STATUS_TYPE_HOT,
			DurationMS: 700, IntervalMS: 200, Potency: 3},
	})
	setup.Units[0].ActiveSkills = []engine.SkillConfig{heal}
	result := runChargeTest(t, setup)
	var ticks []int64
	for _, event := range result.GetEvents() {
		if event.GetStatusId() != "regen" {
			continue
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL {
			ticks = append(ticks, event.GetTick())
			if event.GetAction() != 0 || event.GetAmount() != 3 || event.GetActorId() != "attacker" {
				t.Fatalf("持续治疗来源或数值错误: %v", event)
			}
		}
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED && event.GetTick() != 12 {
			t.Fatal("持续治疗到期时间被辅助 action 改变")
		}
	}
	if !reflect.DeepEqual(ticks, []int64{7, 9, 11}) || len(supportSkillUses(result, "attack")) != 7 {
		t.Fatalf("持续治疗挤占攻击或重复结算: %v", ticks)
	}
}

// TestMixedAndHostileSkillsKeepAttackWindows 伤害附带回血、敌方控制及负面属性仍走正常出手。
func TestMixedAndHostileSkillsKeepAttackWindows(t *testing.T) {
	for _, mode := range []string{"damage-and-heal", "enemy-stun", "negative-attribute"} {
		t.Run(mode, func(t *testing.T) {
			setup := supportTestSetup()
			skill := chargeTestSkill()
			skill.ID, skill.Cooldown = mode, 0
			switch mode {
			case "damage-and-heal":
				skill.Effects = append(skill.Effects, engine.EffectConfig{Kind: pb.EffectType_EFFECT_TYPE_HEAL, Flat: 1})
			case "enemy-stun":
				skill.Effects = []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS,
					Status: &engine.StatusConfig{ID: "control", Kind: pb.StatusType_STATUS_TYPE_STUN, DurationTurns: 2}}}
			case "negative-attribute":
				skill.TargetRule = pb.TargetRule_TARGET_RULE_SELF
				skill.Effects = []engine.EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS,
					Status: &engine.StatusConfig{ID: "debuff", Kind: pb.StatusType_STATUS_TYPE_ATTRIBUTE_MODIFIER,
						DurationTurns: 2, Modifier: engine.AttributeModifier{DefenseFlat: -2}}}}
			}
			setup.Units[0].ActiveSkills = []engine.SkillConfig{skill}
			result := runChargeTest(t, setup)
			uses := supportSkillUses(result, mode)
			if len(uses) != 7 || len(supportSkillUses(result, "attack")) != 0 {
				t.Fatal("伤害或负面效果被误归为额外免费技能")
			}
			for _, event := range uses {
				if event.GetImpactTick() <= event.GetStartTick() || event.GetEndTick() <= event.GetImpactTick() {
					t.Fatalf("攻击或控制技能丢失动作窗口: %v", event)
				}
			}
		})
	}
}

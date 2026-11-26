package engine

import (
	"reflect"
	"sort"
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// testSetup 构建可重复使用的双阵营测试数据。
func testSetup() Setup {
	basic := SkillConfig{ID: "basic", TargetRule: pb.TargetRule_TARGET_RULE_ENEMY_SINGLE, Effects: []EffectConfig{{Kind: pb.EffectType_EFFECT_TYPE_DAMAGE, CoefficientPermille: 1000}}}
	return Setup{BattleID: "battle-1", Seed: 42, Rules: Rules{ATBThreshold: 1000, CritMultiplierPermille: 1500}, Units: []UnitConfig{
		{InstanceID: "a", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, MaxHP: 100, Attack: 30, Defense: 10, Speed: 100, BasicSkill: basic},
		{InstanceID: "d", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, MaxHP: 100, Attack: 20, Defense: 10, Speed: 50, BasicSkill: basic},
	}, Strategies: testStrategies()}
}

// testStrategies 组装引擎测试使用的策略实现。
func testStrategies() Strategies {
	return Strategies{Targets: testTargets{}, Skills: testSkills{}, Effects: testEffects{}, Damage: testDamage{}, Statuses: testStatuses{}}
}

type testTargets struct{}

// Select 按目标规则选取符合条件的单位。
func (testTargets) Select(query TargetQuery) []string {
	candidates := make([]UnitView, 0, len(query.Units))
	for _, unit := range query.Units {
		if !unit.Alive {
			continue
		}
		if query.Rule == pb.TargetRule_TARGET_RULE_SELF && unit.ID == query.Actor.ID {
			return []string{unit.ID}
		}
		if query.Rule == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE || query.Rule == pb.TargetRule_TARGET_RULE_ENEMY_ALL {
			if unit.Team != query.Actor.Team {
				candidates = append(candidates, unit)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Position != candidates[j].Position {
			return candidates[i].Position < candidates[j].Position
		}
		return candidates[i].ID < candidates[j].ID
	})
	if query.Rule == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE && len(candidates) > 1 {
		candidates = candidates[:1]
	}
	ids := make([]string, 0, len(candidates))
	for _, unit := range candidates {
		ids = append(ids, unit.ID)
	}
	return ids
}

type testSkills struct{}

// Execute 逐项结算测试技能的效果。
func (testSkills) Execute(context SkillExecutionContext) ([]EffectResolution, error) {
	results := make([]EffectResolution, 0, len(context.Skill.Effects))
	for _, effect := range context.Skill.Effects {
		result, err := context.Effects.Resolve(EffectContext{Actor: context.Actor, Target: context.Target, Skill: context.Skill, Effect: effect, Rules: context.Rules, Damage: context.Damage, RNG: context.RNG})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

type testEffects struct{}

// Resolve 根据技能效果计算结算结果。
func (testEffects) Resolve(context EffectContext) (EffectResolution, error) {
	result := context.Damage.Calculate(DamageInput{Attack: context.Actor.Attack, Defense: context.Target.Defense, CoefficientPermille: context.Effect.CoefficientPermille, Flat: context.Effect.Flat}, context.RNG)
	return EffectResolution{Kind: pb.EffectType_EFFECT_TYPE_DAMAGE, Amount: result.Amount}, nil
}

// Apply 将结算结果应用到目标。
func (testEffects) Apply(context EffectApplyContext, result EffectResolution) error {
	before := context.Target.HP()
	amount := min(result.Amount, before)
	context.Target.SetHP(before - amount)
	context.Emit(
		pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, "", amount,
		before, context.Target.HP(), result,
	)
	return nil
}

type testDamage struct{}

// Calculate 根据攻防差值和技能系数计算测试伤害。
func (testDamage) Calculate(input DamageInput, _ *RNG) DamageResult {
	amount := (input.Attack-input.Defense)*input.CoefficientPermille/1000 + input.Flat
	if amount < 1 {
		amount = 1
	}
	return DamageResult{Amount: amount}
}

type testStatuses struct{}

// Has 判断指定类型的状态是否仍然有效。
func (testStatuses) Has(statuses []StatusInstance, kind pb.StatusType) bool {
	for _, status := range statuses {
		if status.Remaining > 0 && status.Config.Kind == kind {
			return true
		}
	}
	return false
}

// Modifier 汇总有效状态的属性修正。
func (testStatuses) Modifier([]StatusInstance) AttributeModifier { return AttributeModifier{} }

// Apply 根据配置添加或更新目标状态。
func (testStatuses) Apply(statuses []StatusInstance, config StatusConfig) ([]StatusInstance, error) {
	return append(statuses, StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}

// Tick 计算一次周期状态触发的结果。
func (testStatuses) Tick(StatusInstance) StatusTickResult { return StatusTickResult{} }

// Expire 更新状态剩余回合并移除已失效的状态。
func (testStatuses) Expire(statuses []StatusInstance) []StatusInstance {
	kept := statuses[:0]
	for _, status := range statuses {
		status.Remaining--
		if status.Remaining > 0 {
			kept = append(kept, status)
		}
	}
	return kept
}

// TestBattleDeterministicResult 验证相同输入和种子产生相同战斗结果。
func TestBattleDeterministicResult(t *testing.T) {
	setup := testSetup()
	setup.Units[1].MaxHP = 1
	first, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	firstResult, err := finishWithBasicAttack(t, first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := finishWithBasicAttack(t, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.Checksum != secondResult.Checksum || !reflect.DeepEqual(firstResult.Events, secondResult.Events) {
		t.Fatalf("results are not deterministic: %#v %#v", firstResult, secondResult)
	}
	ended := firstResult.Events[len(firstResult.Events)-1]
	if ended.GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_BATTLE_ENDED || ended.GetOutcome() != firstResult.Outcome {
		t.Fatalf("战斗结束事件未携带协议胜负结果: %#v", ended)
	}
}

// TestBattleFinalizeIsIdempotent 锁定 Finalize 的幂等语义：重复调用返回同一结果，
// 且不会重复追加 BattleEnded 事件。
func TestBattleFinalizeIsIdempotent(t *testing.T) {
	setup := testSetup()
	setup.Units[1].MaxHP = 1
	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	first, err := finishWithBasicAttack(t, battle)
	if err != nil {
		t.Fatal(err)
	}
	second, err := battle.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.Checksum != second.Checksum || len(first.Events) != len(second.Events) {
		t.Fatalf("Finalize 不幂等: first=%d events second=%d events", len(first.Events), len(second.Events))
	}
}

// TestBattleRequiresBothTeams 验证缺少任一方阵营时无法创建战斗。
func TestBattleRequiresBothTeams(t *testing.T) {
	setup := testSetup()
	setup.Units = setup.Units[:1]
	if _, err := NewBattle(setup); err == nil {
		t.Fatal("expected validation error")
	}
}

// TestBasicAttackEventMarkedForCollisionPresentation 验证普攻事件包含碰撞表现所需的标记。
func TestBasicAttackEventMarkedForCollisionPresentation(t *testing.T) {
	setup := testSetup()
	setup.Units[1].MaxHP = 1
	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := finishWithBasicAttack(t, battle)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range result.Events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED {
			if !event.GetBasicAttack() {
				t.Fatalf("普攻事件缺少 basic_attack 标识: %#v", event)
			}
			return
		}
	}
	t.Fatal("battle emitted no skill-used event")
}

// finishWithBasicAttack 通过玩家输入击败低血量目标，再读取正式结算结果。
func finishWithBasicAttack(t *testing.T, b *Battle) (Result, error) {
	t.Helper()
	ack := b.ApplyInput(&pb.BattleInputReq{BattleId: b.ID(), UnitId: "a", SkillId: "basic", Sequence: 1}, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	if ack.Action == 0 || !b.InputDone() {
		t.Fatalf("普攻未结束战斗: %v", ack)
	}
	return b.Finalize()
}

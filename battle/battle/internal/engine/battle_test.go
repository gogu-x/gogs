package engine

import (
	"reflect"
	"sort"
	"testing"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func testSetup() Setup {
	basic := SkillConfig{ID: "basic", TargetRule: pb.TargetRule_TARGET_RULE_ENEMY_SINGLE, Effects: []EffectConfig{{Kind: effectkind.Damage, CoefficientPermille: 1000}}}
	return Setup{BattleID: "battle-1", Seed: 42, Rules: Rules{ATBThreshold: 1000, MaxActions: 20, CritMultiplierPermille: 1500}, Units: []UnitConfig{
		{InstanceID: "a", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, MaxHP: 100, Attack: 30, Defense: 10, Speed: 100, BasicSkill: basic},
		{InstanceID: "d", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, MaxHP: 100, Attack: 20, Defense: 10, Speed: 50, BasicSkill: basic},
	}, Strategies: testStrategies()}
}

func testStrategies() Strategies {
	return Strategies{Targets: testTargets{}, Skills: testSkills{}, Effects: testEffects{}, Damage: testDamage{}, Statuses: testStatuses{}}
}

type testTargets struct{}

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

func (testEffects) Resolve(context EffectContext) (EffectResolution, error) {
	result := context.Damage.Calculate(DamageInput{Attack: context.Actor.Attack, Defense: context.Target.Defense, CoefficientPermille: context.Effect.CoefficientPermille, Flat: context.Effect.Flat}, context.RNG)
	return EffectResolution{Kind: effectkind.Damage, Amount: result.Amount}, nil
}

func (testEffects) Apply(context EffectApplyContext, result EffectResolution) error {
	before := context.Target.HP()
	amount := min(result.Amount, before)
	context.Target.SetHP(before - amount)
	context.Emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, "", amount, before, context.Target.HP(), "")
	return nil
}

type testDamage struct{}

func (testDamage) Calculate(input DamageInput, _ *RNG) DamageResult {
	amount := (input.Attack-input.Defense)*input.CoefficientPermille/1000 + input.Flat
	if amount < 1 {
		amount = 1
	}
	return DamageResult{Amount: amount}
}

type testStatuses struct{}

func (testStatuses) Has(statuses []StatusInstance, kind statuskind.Kind) bool {
	for _, status := range statuses {
		if status.Remaining > 0 && status.Config.Kind == kind {
			return true
		}
	}
	return false
}
func (testStatuses) Modifier([]StatusInstance) AttributeModifier { return AttributeModifier{} }
func (testStatuses) Apply(statuses []StatusInstance, config StatusConfig) ([]StatusInstance, error) {
	return append(statuses, StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}
func (testStatuses) Tick(StatusInstance) StatusTickResult { return StatusTickResult{} }
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

func TestBattleDeterministicResult(t *testing.T) {
	setup := testSetup()
	first, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	firstResult, err := first.Run()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := second.Run()
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.Checksum != secondResult.Checksum || !reflect.DeepEqual(firstResult.Events, secondResult.Events) {
		t.Fatalf("results are not deterministic: %#v %#v", firstResult, secondResult)
	}
}

// TestBattleAdvanceMatchesRun 是流式推送改造的等价性守卫：按 tick 分批推进（NextTick/Advance/Flush）
// 必须与一次性 Run 产生完全相同的事件流与校验和。任何 tick 相位、事件顺序或终止时机的漂移
// 都会被这里捕获。
func TestBattleAdvanceMatchesRun(t *testing.T) {
	setup := testSetup()
	batch, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}

	var streamed []*pb.BattleEvent
	batches := 0
	for {
		next, ok := batch.NextTick()
		if !ok {
			break
		}
		if next <= 0 {
			t.Fatalf("NextTick 返回了非正 tick: %d", next)
		}
		// Advance 只返回当前 tick 的事件，不得越界到后续 tick。
		events := batch.Advance()
		for _, event := range events {
			if event.Tick != next {
				t.Fatalf("Advance 越过了 tick 边界: 期望 %d, 实际 %d", next, event.Tick)
			}
		}
		streamed = append(streamed, events...)
		batches++
	}
	batchResult, err := batch.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	// Finalize 会产生收尾的 BattleEnded 事件，必须能通过 Flush 取到。
	streamed = append(streamed, batch.Flush()...)

	oneShot, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	oneShotResult, err := oneShot.Run()
	if err != nil {
		t.Fatal(err)
	}

	if batchResult.Checksum != oneShotResult.Checksum {
		t.Fatalf("checksum 不一致: advance=%s run=%s", batchResult.Checksum, oneShotResult.Checksum)
	}
	if !reflect.DeepEqual(batchResult.Events, oneShotResult.Events) {
		t.Fatalf("结果事件流不一致:\nadvance=%#v\nrun=%#v", batchResult.Events, oneShotResult.Events)
	}
	if !reflect.DeepEqual(streamed, oneShotResult.Events) {
		t.Fatalf("分批推送的事件与完整事件流不一致:\nstreamed=%#v\nrun=%#v", streamed, oneShotResult.Events)
	}
	if batches < 2 {
		t.Fatalf("期望产生多个 tick 批次,实际只有 %d 个", batches)
	}
	t.Logf("按 tick 分批: batches=%d events=%d", batches, len(streamed))
}

// TestSkillUsedCarriesMotionWindow 锁定客户端契约：SKILL_USED 事件必须自带
// start/impact/end 三个 tick，且与随后真正到达的 DAMAGE / ACTION_ENDED 完全一致。
// 客户端据此可以在收到单条事件时立刻起播攻击动作，不必等待动作结束的 ACTION_ENDED。
func TestSkillUsedCarriesMotionWindow(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxActions = 4
	setup.Units[0].Speed = 100
	setup.Units[1].Speed = 100
	setup.Units[0].BasicSkill.WindupTicks = 3
	setup.Units[0].BasicSkill.RecoveryTicks = 2
	setup.Units[1].BasicSkill.WindupTicks = 3
	setup.Units[1].BasicSkill.RecoveryTicks = 2

	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, event := range result.Events {
		if event.Type != pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED {
			continue
		}
		if event.StartTick <= 0 || event.ImpactTick <= 0 || event.EndTick <= 0 {
			t.Fatalf("action %d 的 SKILL_USED 未携带完整窗口: %#v", event.Action, event)
		}
		if event.StartTick != event.Tick {
			t.Fatalf("action %d: start_tick=%d 与 tick=%d 不一致", event.Action, event.StartTick, event.Tick)
		}
		if !(event.StartTick < event.ImpactTick && event.ImpactTick < event.EndTick) {
			t.Fatalf("action %d: 窗口顺序非法 %d/%d/%d", event.Action, event.StartTick, event.ImpactTick, event.EndTick)
		}
		if end := eventOfType(result.Events, event.Action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED); end == nil {
			t.Fatalf("action %d: 缺少 ACTION_ENDED", event.Action)
		} else if end.Tick != event.EndTick {
			t.Fatalf("action %d: end.Tick=%d 与 end_tick=%d 不一致", event.Action, end.Tick, event.EndTick)
		}
		// 被取消的动作没有 DAMAGE，但窗口本身仍须指向 impact tick。
		if damage := eventOfType(result.Events, event.Action, pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE); damage != nil && damage.Tick != event.ImpactTick {
			t.Fatalf("action %d: damage.Tick=%d 与 impact_tick=%d 不一致", event.Action, damage.Tick, event.ImpactTick)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("没有任何 SKILL_USED 事件被检查")
	}
}

// TestBattleFinalizeIsIdempotent 锁定 Finalize 的幂等语义：重复调用返回同一结果，
// 且不会重复追加 BattleEnded 事件。
func TestBattleFinalizeIsIdempotent(t *testing.T) {
	battle, err := NewBattle(testSetup())
	if err != nil {
		t.Fatal(err)
	}
	first, err := battle.Run()
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

func TestBattleRequiresBothTeams(t *testing.T) {
	setup := testSetup()
	setup.Units = setup.Units[:1]
	if _, err := NewBattle(setup); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBasicAttackEventMarkedForCollisionPresentation(t *testing.T) {
	battle, err := NewBattle(testSetup())
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range result.Events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED {
			if event.GetDetail() != "basic" {
				t.Fatalf("basic attack detail = %q, want basic", event.GetDetail())
			}
			return
		}
	}
	t.Fatal("battle emitted no skill-used event")
}

func eventOfType(events []*pb.BattleEvent, action uint64, kind pb.BattleEventType) *pb.BattleEvent {
	for _, event := range events {
		if event.GetAction() == action && event.GetType() == kind {
			return event
		}
	}
	return nil
}

func TestTimelineAllowsDifferentUnitsToOverlap(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxActions = 4
	setup.Units[0].Speed = 100
	setup.Units[1].Speed = 100
	setup.Units[0].BasicSkill.WindupTicks = 3
	setup.Units[0].BasicSkill.RecoveryTicks = 2
	setup.Units[1].BasicSkill.WindupTicks = 3
	setup.Units[1].BasicSkill.RecoveryTicks = 2

	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	firstStart := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED)
	secondStart := eventOfType(result.Events, 2, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED)
	firstDamage := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE)
	secondDamage := eventOfType(result.Events, 2, pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE)
	firstEnd := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED)
	if firstStart == nil || secondStart == nil || firstDamage == nil || secondDamage == nil || firstEnd == nil {
		t.Fatalf("missing phased events: %#v", result.Events)
	}
	if firstStart.Tick != secondStart.Tick || secondStart.Tick >= firstEnd.Tick {
		t.Fatalf("actions did not overlap: first=%d..%d second=%d", firstStart.Tick, firstEnd.Tick, secondStart.Tick)
	}
	if firstDamage.Tick != firstStart.Tick+3 || firstEnd.Tick != firstDamage.Tick+2 {
		t.Fatalf("unexpected phases: start=%d impact=%d end=%d", firstStart.Tick, firstDamage.Tick, firstEnd.Tick)
	}
	if secondDamage.Tick != firstDamage.Tick || secondDamage.Sequence <= firstDamage.Sequence {
		t.Fatalf("same-tick impact order is unstable: first=%#v second=%#v", firstDamage, secondDamage)
	}
	if len(firstStart.TargetIds) != 1 || firstStart.TargetIds[0] == "" {
		t.Fatalf("ActionStarted target_ids = %#v", firstStart.TargetIds)
	}

	lastEndByActor := map[string]int64{}
	for _, event := range result.Events {
		switch event.Type {
		case pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED:
			if end, exists := lastEndByActor[event.ActorId]; exists && event.Tick < end {
				t.Fatalf("actor %s restarted at %d before end %d", event.ActorId, event.Tick, end)
			}
		case pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED:
			lastEndByActor[event.ActorId] = event.Tick
		}
	}
}

func TestMeleeImpactCancelledWhenAttackerDiesDuringWindup(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxActions = 2
	setup.Units[0].MaxHP = 20
	setup.Units[0].Speed = 100
	setup.Units[0].BasicSkill.WindupTicks = 5
	setup.Units[1].Attack = 100
	setup.Units[1].Speed = 100
	setup.Units[1].BasicSkill.WindupTicks = 1

	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	cancelled := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED)
	if cancelled == nil || cancelled.Detail != "attacker_dead" || cancelled.Tick != 15 {
		t.Fatalf("attacker cancellation = %#v", cancelled)
	}
	if damage := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE); damage != nil {
		t.Fatalf("dead melee attacker still dealt damage: %#v", damage)
	}
}

func TestImpactCancelledWhenSnapshottedTargetDies(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxActions = 3
	setup.Units[0].Attack = 200
	setup.Units[0].Speed = 100
	setup.Units[0].BasicSkill.WindupTicks = 1
	second := setup.Units[0]
	second.InstanceID = "b"
	second.Position = 2
	second.Attack = 30
	second.BasicSkill.WindupTicks = 3
	setup.Units = append([]UnitConfig{setup.Units[0], second}, setup.Units[1])
	setup.Units[2].Position = 3
	setup.Units[2].Speed = 100
	setup.Units[2].BasicSkill.WindupTicks = 4

	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	cancelled := eventOfType(result.Events, 2, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED)
	if cancelled == nil || cancelled.Detail != "target_dead" || cancelled.TargetId != "d" {
		t.Fatalf("target cancellation = %#v", cancelled)
	}
}

func TestLegacyTimingDefaults(t *testing.T) {
	battle, err := NewBattle(testSetup())
	if err != nil {
		t.Fatal(err)
	}
	if battle.TickDurationMS() != DefaultTickDurationMS {
		t.Fatalf("tick duration = %d, want %d", battle.TickDurationMS(), DefaultTickDurationMS)
	}
}

func TestActiveSkillImpactCancelledWhenAttackerDiesDuringWindup(t *testing.T) {
	setup := testSetup()
	setup.Rules.MaxActions = 2
	setup.Units[0].MaxHP = 20
	setup.Units[0].Speed = 100
	active := setup.Units[0].BasicSkill
	active.ID = "active"
	active.WindupTicks = 5
	active.Cooldown = 2
	setup.Units[0].ActiveSkills = []SkillConfig{active}
	setup.Units[1].Attack = 100
	setup.Units[1].Speed = 100
	setup.Units[1].BasicSkill.WindupTicks = 1

	battle, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	cancelled := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED)
	if cancelled == nil || cancelled.Detail != "attacker_dead" || cancelled.SkillId != "active" {
		t.Fatalf("active skill cancellation = %#v", cancelled)
	}
	if damage := eventOfType(result.Events, 1, pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE); damage != nil {
		t.Fatalf("dead active-skill attacker still dealt damage: %#v", damage)
	}
}

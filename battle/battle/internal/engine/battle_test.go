package engine

import (
	"reflect"
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func testSetup() Setup {
	basic := SkillConfig{ID: "basic", TargetRule: pb.TargetRule_TARGET_RULE_ENEMY_SINGLE, Effects: []EffectConfig{{Kind: EffectDamage, CoefficientPermille: 1000}}}
	return Setup{BattleID: "battle-1", Seed: 42, Rules: Rules{ATBThreshold: 1000, MaxActions: 20, CritMultiplierPermille: 1500}, Units: []UnitConfig{
		{InstanceID: "a", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, MaxHP: 100, Attack: 30, Defense: 10, Speed: 100, BasicSkill: basic},
		{InstanceID: "d", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, MaxHP: 100, Attack: 20, Defense: 10, Speed: 50, BasicSkill: basic},
	}}
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

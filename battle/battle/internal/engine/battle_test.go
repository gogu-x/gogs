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

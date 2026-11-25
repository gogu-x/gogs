package strategy

import (
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func TestCastPriorityAndTarget(t *testing.T) {
	rules, err := Compile([]*common.TypIDVal{
		{Typ: "ally_hp", Id: 4, Val: 650}, {Typ: "enemy_hp", Id: 3, Val: 400}, {Typ: "enemy_count", Id: 2, Val: 2},
	}, map[string]bool{"4": true, "3": true, "2": true})
	if err != nil {
		t.Fatal(err)
	}
	control := &pb.BattleControl{UnitId: "caster", BasicSkill: "11"}
	units := []*pb.BattleUnitResult{
		{InstanceId: "caster", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Alive: true, Hp: 100, MaxHp: 100},
		{InstanceId: "ally", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Alive: true, Hp: 65, MaxHp: 100},
		{InstanceId: "one", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Alive: true, Hp: 41, MaxHp: 100},
		{InstanceId: "two", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Alive: true, Hp: 80, MaxHp: 200},
	}
	available := map[string]pb.TargetRule{"4": pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP, "3": pb.TargetRule_TARGET_RULE_ENEMY_SINGLE, "2": pb.TargetRule_TARGET_RULE_ENEMY_ALL}
	check := func(wantSkill, wantTarget string) {
		t.Helper()
		skill, target := Select(control, rules, units, available)
		if skill != wantSkill || target != wantTarget {
			t.Fatalf("得到 (%s,%s)，期望 (%s,%s)", skill, target, wantSkill, wantTarget)
		}
	}
	check("4", "")
	delete(available, "4")
	check("3", "two")
	units[3].Hp = 100
	check("2", "")
	units[2].Alive = false
	check("11", "")
}

func TestInvalidRules(t *testing.T) {
	for _, rule := range []*common.TypIDVal{{Typ: "unknown", Id: 1}, {Typ: "self_hp", Id: 1, Val: 1001}, {Typ: "always", Id: 7}, {Typ: "enemy_count", Id: 1}, nil} {
		if _, err := Compile([]*common.TypIDVal{rule}, map[string]bool{"1": true}); err == nil {
			t.Fatalf("无效规则被接受 %v", rule)
		}
	}
}

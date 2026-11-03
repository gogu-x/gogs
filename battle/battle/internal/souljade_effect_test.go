package internal

import (
	"testing"

	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func installSoulJadeEffectConfigs(t *testing.T) {
	t.Helper()
	if err := glconf.SetSoulJadeEffectAffixCfgForTest([]*glconf.SoulJadeEffectAffixCfg{
		{CfgID: 1001, Name: "冰爆", EffectType: int32(pb.EffectType_EFFECT_TYPE_ICE_BURST), RollWeight: 1, TriggerThreshold: 3, GainPerHit: 1, DamageCoefficientPermille: 1500, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_ALL)},
		{CfgID: 1002, Name: "天雷", EffectType: int32(pb.EffectType_EFFECT_TYPE_THUNDER), RollWeight: 1, TriggerThreshold: 3, GainPerHit: 1, DamageCoefficientPermille: 1500, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_ALL)},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSoulJadeEffectAffixesTriggerForEquippedRole(t *testing.T) {
	installFactoryConfigs(t)
	installSoulJadeEffectConfigs(t)
	role := battleRole("attacker", 1)
	role.SoulJadeEffectAffixIds = []int32{1001, 1002}
	unit, err := buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit.PassiveCharges) != 2 || unit.PassiveCharges[0].Threshold != 3 || unit.PassiveCharges[1].Gain != 1 {
		t.Fatalf("魂玉效果未按角色编译: %#v", unit.PassiveCharges)
	}
	request := &pb.StartBattleReq{
		BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{role}, MonsterGroupConfigId: 1,
	}
	battle, err := newBattleFromRequest("souljade-effects", 17, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[pb.EffectType]bool)
	for _, event := range result.Events {
		if event.GetActorId() == "attacker" && event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			seen[event.GetEffectType()] = true
		}
	}
	if !seen[pb.EffectType_EFFECT_TYPE_ICE_BURST] || !seen[pb.EffectType_EFFECT_TYPE_THUNDER] {
		t.Fatalf("魂玉效果未触发: %#v", seen)
	}
}

func TestSoulJadeEffectAffixRejectsUnknownID(t *testing.T) {
	installFactoryConfigs(t)
	installSoulJadeEffectConfigs(t)
	role := battleRole("attacker", 1)
	role.SoulJadeEffectAffixIds = []int32{9999}
	if _, err := buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_ATTACKER); err == nil {
		t.Fatal("未知魂玉效果词条 ID 未被拒绝")
	}
}

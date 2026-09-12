package internal

import (
	"strings"
	"testing"

	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	cspb "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func installFactoryConfigs(t *testing.T) {
	t.Helper()
	err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{
			{CfgID: 1, MaxHP: 100, Attack: 20, Defense: 5, Speed: 10, DefaultPosition: 1, BasicSkillID: 1},
			{CfgID: 2, MaxHP: 120, Attack: 15, Defense: 8, Speed: 8, DefaultPosition: 2, BasicSkillID: 1},
		},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), Effects: []*cspb.TypIDVal{{Typ: battleDamageType, Pro: 1000}}}},
		[]*glconf.BattleEquipCfg{{CfgID: 1, Modifier: []*cspb.TypIDVal{{Typ: battleAttributeType, Id: battleAttributeAttack, Val: 5}}}},
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 80, Attack: 12, Defense: 4, Speed: 6, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*cspb.TypIDVal{{Typ: battleMonsterType, Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{
			{BattleType: int32(pb.BattleType_BATTLE_TYPE_PVE), ATBThreshold: 1000, MaxActions: 20, CritMultiplierPermille: 1500},
			{BattleType: int32(pb.BattleType_BATTLE_TYPE_PVP), ATBThreshold: 1000, MaxActions: 20, CritMultiplierPermille: 1500},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetBattleStatusConfigsForTest([]*glconf.BattleStatusCfg{{CfgID: 1, Kind: int32(5), DurationTurns: 2, Modifier: []*cspb.TypIDVal{{Typ: battleAttributeType, Id: battleAttributeAttack, Val: 3}}}}); err != nil {
		t.Fatal(err)
	}
}

func battleRole(id string, configID int32) *pb.Role {
	return &pb.Role{RoleId: id, RoleConfigId: configID, Level: 1}
}

func TestNewBattleFromRequestPVE(t *testing.T) {
	installFactoryConfigs(t)
	request := &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVE, AttackerRoles: []*pb.Role{battleRole("a1", 1), battleRole("a2", 2)}, MonsterGroupConfigId: 1}
	battle, err := newBattleFromRequest("battle-pve", 7, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil {
		t.Fatal(err)
	}
	if result.GetBattleID() != "battle-pve" || len(result.GetUnits()) != 3 || len(result.GetEvents()) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestNewBattleFromRequestPVP(t *testing.T) {
	installFactoryConfigs(t)
	request := &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVP, AttackerRoles: []*pb.Role{battleRole("a", 1)}, DefenderRoles: []*pb.Role{battleRole("d", 2)}}
	battle, err := newBattleFromRequest("battle-pvp", 9, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := battle.Run()
	if err != nil || len(result.GetUnits()) != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestValidateBattleRequestTable(t *testing.T) {
	tests := []struct {
		name    string
		request *pb.StartBattleReq
		wantErr string
	}{
		{name: "pve missing group", request: &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVE, AttackerRoles: []*pb.Role{battleRole("a", 1)}}, wantErr: "requires monster group"},
		{name: "pve has defender", request: &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVE, AttackerRoles: []*pb.Role{battleRole("a", 1)}, DefenderRoles: []*pb.Role{battleRole("d", 2)}, MonsterGroupConfigId: 1}, wantErr: "forbids defender"},
		{name: "pvp multiple attackers", request: &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVP, AttackerRoles: []*pb.Role{battleRole("a", 1), battleRole("a2", 2)}, DefenderRoles: []*pb.Role{battleRole("d", 2)}}, wantErr: "exactly one"},
		{name: "pvp has group", request: &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_PVP, AttackerRoles: []*pb.Role{battleRole("a", 1)}, DefenderRoles: []*pb.Role{battleRole("d", 2)}, MonsterGroupConfigId: 1}, wantErr: "forbids monster"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBattleRequest(tt.request)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%v want=%q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildRoleUnitUsesConfiguredInitialStatus(t *testing.T) {
	installFactoryConfigs(t)
	config := glconf.GetBattleRoleCfg(1)
	config.InitialStatusIDs = []int32{1}
	unit, err := buildRoleUnit(battleRole("a", 1), pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit.InitialStatus) != 1 || unit.InitialStatus[0].ID != "1" {
		t.Fatalf("initial statuses = %#v", unit.InitialStatus)
	}
}

func TestBuildSkillUsesVisibleTimingDefaults(t *testing.T) {
	installFactoryConfigs(t)
	skill, err := buildSkill(1)
	if err != nil {
		t.Fatal(err)
	}
	if skill.WindupTicks != defaultSkillWindupTicks || skill.RecoveryTicks != defaultSkillRecoveryTicks {
		t.Fatalf("skill timing = %d/%d, want %d/%d", skill.WindupTicks, skill.RecoveryTicks,
			defaultSkillWindupTicks, defaultSkillRecoveryTicks)
	}
}

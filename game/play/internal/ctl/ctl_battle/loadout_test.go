package ctl_battle

import (
	"testing"

	battleengine "github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

func TestBattleIgnoresClientGrowthAndUsesStoredLoadout(t *testing.T) {
	loadouttest.Load(t)
	p := player.NewPlayerData(99)
	p.SoulJadeMgr = souljade.New(99)
	p.TalentMgr = talent.New()
	p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-99", ConfigID: 1, Level: 1})
	if code := talent.Learn(p.GetTalentMgr(), 1, 6); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	incoming := &pb.StartBattleReq{AttackerRoles: []*pb.Role{{RoleId: "forged", GrowthBonus: &pb_common.GrowthBonuses{AttackPermille: 999999}}}}
	request, code := buildBattleRequest(p, incoming)
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	roles := request.GetAttackerRoles()
	if len(roles) != 1 || roles[0].GetRoleId() != "role-99" || roles[0].GetGrowthBonus().GetAttackPermille() != 44 {
		t.Fatalf("battle did not use authoritative growth: %v", roles)
	}
	if incoming.GetAttackerRoles()[0].GetGrowthBonus().GetAttackPermille() != 999999 {
		t.Fatal("original request was modified")
	}
}

func battleLoadoutPlayer(t *testing.T) *player.Player {
	t.Helper()
	loadouttest.Load(t)
	if err := glconf.SetEquipmentSlotCfgForTest([]*glconf.EquipmentSlotCfg{{CfgID: 17, Name: "Weapon"}}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{{CfgID: 1, MaxHP: 1000, Attack: 1000, Defense: 1000, Speed: 1000, DefaultPosition: 1, BasicSkillID: 1}},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), WindupTicks: 1, RecoveryTicks: 1, Effects: []*pb_common.TypIDVal{{Typ: "battle_damage", Pro: 1000}}}},
		[]*glconf.BattleEquipCfg{
			{CfgID: 77, Slot: 17, Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 20}, {Typ: "battle_attribute", Id: 2, Val: 100}, {Typ: "battle_attribute", Id: 3, Val: 200}, {Typ: "battle_attribute", Id: 4, Val: 300}}},
			{CfgID: 91, Slot: 17, Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 200}, {Typ: "battle_attribute", Id: 2, Val: 200}, {Typ: "battle_attribute", Id: 3, Val: 300}, {Typ: "battle_attribute", Id: 4, Val: 400}}},
			{CfgID: 123, Slot: 17, Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 999999}}},
		},
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 1000, Attack: 100, Defense: 100, Speed: 100, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*pb_common.TypIDVal{{Typ: "battle_monster", Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_TOWER), ATBThreshold: 1000, MaxActions: 20, CritMultiplierPermille: 1500}},
	); err != nil {
		t.Fatal(err)
	}
	p := player.NewPlayerData(99)
	p.SoulJadeMgr = souljade.New(99)
	p.TalentMgr = talent.New()
	p.GetRoleMgr().SetCharacter(&role.Role{ID: "role-99", ConfigID: 1, Level: 1, EquipmentIDs: []string{"blade-a"}})
	p.GetEquipmentMgr().AddItem(&equipment.Item{ID: "blade-a", ConfigID: 77})
	p.GetEquipmentMgr().AddItem(&equipment.Item{ID: "blade-b", ConfigID: 91})
	p.GetEquipmentMgr().AddItem(&equipment.Item{ID: "unequipped", ConfigID: 123})
	return p
}

func compileLoadoutAttributes(t *testing.T, request *pb.StartBattleReq) [4]int64 {
	t.Helper()
	setup, err := battleengine.NewDefaultConfigCompiler().Compile("loadout-integration", 7, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range setup.Units {
		if unit.Team == pb.BattleTeam_BATTLE_TEAM_ATTACKER {
			return [4]int64{unit.MaxHP, unit.Attack, unit.Defense, unit.Speed}
		}
	}
	t.Fatal("compiled setup omitted the authoritative player")
	return [4]int64{}
}

func TestNextBattleUsesUpdatedLoadoutWithoutChangingExistingRequest(t *testing.T) {
	p := battleLoadoutPlayer(t)
	request, code := buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	original := proto.Clone(request)
	initial := [4]int64{1020, 1139, 1228, 1315}
	if actual := compileLoadoutAttributes(t, request); actual != initial {
		t.Fatalf("initial HP/ATK/DEF/SPD = %v, want %v; unequipped inventory must not contribute", actual, initial)
	}
	if code := equipment.ChangeRoleEquipment(
		p.GetRoleMgr(), p.GetEquipmentMgr(), 17, "blade-b",
	); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	next, code := buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if actual := compileLoadoutAttributes(t, next); actual != [4]int64{1200, 1243, 1331, 1416} {
		t.Fatalf("equipment replacement did not reach the next battle: %v", actual)
	}
	jadeMgr := p.GetSoulJadeMgr()
	var healthJade string
	for id, item := range jadeMgr.GetItems() {
		if item.GetConfigID() == 8 {
			healthJade = id
		}
	}
	if healthJade == "" {
		t.Fatal("configured unequipped health jade is missing")
	}
	if code := souljade.Change(jadeMgr, 1, healthJade); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	next, code = buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if actual := compileLoadoutAttributes(t, next); actual != [4]int64{1214, 1228, 1331, 1416} {
		t.Fatalf("soul jade replacement did not remove the old modifier and add the new one: %v", actual)
	}
	for _, nodeID := range []int32{1, 25, 49, 73} {
		if code := talent.Learn(p.GetTalentMgr(), nodeID, 6); code != pb_common.ErrCode_OK {
			t.Fatal(code)
		}
	}
	next, code = buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if actual := compileLoadoutAttributes(t, next); actual != [4]int64{1224, 1238, 1341, 1428} {
		t.Fatalf("learned talent modifiers did not reach all four battle attributes: %v", actual)
	}
	if !proto.Equal(request, original) {
		t.Fatal("later loadout changes mutated the existing battle request")
	}
	if actual := compileLoadoutAttributes(t, request); actual != initial {
		t.Fatalf("existing battle request no longer compiles to its original attributes: %v", actual)
	}
}

func TestBattleRejectsClientLoadoutAndLocalProgressionInputs(t *testing.T) {
	p := battleLoadoutPlayer(t)
	clean, code := buildBattleRequest(p, &pb.StartBattleReq{})
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	forged := &pb.StartBattleReq{
		UID: 999, BattleType: pb.BattleType_BATTLE_TYPE_PVP, MonsterGroupConfigId: 999,
		AttackerRoles: []*pb.Role{{RoleId: "forged", RoleConfigId: 999, Level: 999, Star: 999, Breakthrough: 999,
			Equips:      []*pb.RoleEquip{{EquipId: "unequipped", EquipConfigId: 123, Level: 999, RefineLevel: 999}},
			Skills:      []*pb.RoleSkill{{SkillConfigId: 999, Level: 999}},
			GrowthBonus: &pb_common.GrowthBonuses{MaxHpPermille: 999999, AttackPermille: 999999, DefensePermille: 999999, SpeedPermille: 999999}}},
		DefenderRoles: []*pb.Role{{RoleId: "forged-enemy", RoleConfigId: 999}},
	}
	before := proto.Clone(forged)
	actual, code := buildBattleRequest(p, forged)
	if code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if !proto.Equal(actual, clean) {
		t.Fatal("client equipment, progression or growth values entered the authoritative battle request")
	}
	if !proto.Equal(forged, before) {
		t.Fatal("server assembly mutated the original client packet")
	}
}

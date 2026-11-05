package configcompiler

import (
	"math"
	"testing"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func TestGrowthAppliesAfterEquipmentExactlyOnce(t *testing.T) {
	role := &glconf.BattleRoleCfg{MaxHP: 1000, Attack: 100, Defense: 50, Speed: 100}
	equips := []*glconf.BattleEquipCfg{{Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 200}, {Typ: "battle_attribute", Id: 2, Val: 20}}}}
	state := &pb.Role{GrowthBonus: &pb_common.GrowthBonuses{MaxHpPermille: 100, AttackPermille: 250, DefensePermille: 200, SpeedPermille: 100}}
	actual, err := (configuredRoleStatCalculator{}).Calculate(state, role, equips)
	if err != nil {
		t.Fatal(err)
	}
	if actual.GetMaxHP() != 1320 || actual.GetAttack() != 150 || actual.GetDefense() != 60 || actual.GetSpeed() != 110 {
		t.Fatalf("incorrect equipment or growth calculation: %+v", actual)
	}
	base, err := (configuredRoleStatCalculator{}).Calculate(&pb.Role{}, role, equips)
	if err != nil || base.GetMaxHP() != 1200 || base.GetAttack() != 120 {
		t.Fatal("zero-growth compatibility was broken")
	}
}

func TestCompiledBattleIgnoresLaterRequestAndConfigurationMutations(t *testing.T) {
	roleConfig := &glconf.BattleRoleCfg{CfgID: 1, MaxHP: 1000, Attack: 100, Defense: 50, Speed: 100, DefaultPosition: 1, BasicSkillID: 1}
	equipConfig := &glconf.BattleEquipCfg{CfgID: 77, Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 200}, {Typ: "battle_attribute", Id: 2, Val: 20}, {Typ: "battle_attribute", Id: 3, Val: 10}, {Typ: "battle_attribute", Id: 4, Val: 5}}}
	if err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{roleConfig},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), WindupTicks: 1, RecoveryTicks: 1, Effects: []*pb_common.TypIDVal{{Typ: "battle_damage", Pro: 1000}}}},
		[]*glconf.BattleEquipCfg{equipConfig},
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 1000, Attack: 100, Defense: 50, Speed: 100, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*pb_common.TypIDVal{{Typ: "battle_monster", Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_TOWER), ATBThreshold: 1000, MaxActions: 30, CritMultiplierPermille: 1500}},
	); err != nil {
		t.Fatal(err)
	}
	role := &pb.Role{RoleId: "player", RoleConfigId: 1, Equips: []*pb.RoleEquip{{EquipId: "blade", EquipConfigId: 77}},
		GrowthBonus: &pb_common.GrowthBonuses{MaxHpPermille: 100, AttackPermille: 250, DefensePermille: 200, SpeedPermille: 100}}
	request := &pb.StartBattleReq{BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{role}, MonsterGroupConfigId: 1}
	compiler := NewDefault()
	setup, err := compiler.Compile("snapshot-isolation", 7, request)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := compiler.CalculateRoleAttributes(role)
	if err != nil {
		t.Fatal(err)
	}
	compiled := setup.Units[0]
	if actual := [4]int64{compiled.MaxHP, compiled.Attack, compiled.Defense, compiled.Speed}; actual != [4]int64{preview.GetMaxHP(), preview.GetAttack(), preview.GetDefense(), preview.GetSpeed()} {
		t.Fatalf("read-only attributes differ from actual battle compilation: %v", actual)
	}
	control, err := engine.NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := control.Run()
	if err != nil {
		t.Fatal(err)
	}
	running, err := engine.NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	running.Advance()
	role.GrowthBonus = &pb_common.GrowthBonuses{MaxHpPermille: 5000, AttackPermille: 5000, DefensePermille: 5000, SpeedPermille: 5000}
	role.Equips = nil
	roleConfig.MaxHP = 9999
	roleConfig.Attack = 9999
	roleConfig.Defense = 9999
	roleConfig.Speed = 9999
	equipConfig.Modifier[0].Val = 9999
	actual, err := running.Run()
	if err != nil {
		t.Fatal(err)
	}
	if actual.GetChecksum() != expected.GetChecksum() {
		t.Fatal("an existing battle observed later request or configuration changes")
	}
	next, err := compiler.Compile("next-battle", 8, request)
	if err != nil {
		t.Fatal(err)
	}
	if next.Units[0].Attack == compiled.Attack {
		t.Fatal("a new battle did not compile updated input")
	}
}

func TestGrowthRejectsInvalidValuesAndOverflow(t *testing.T) {
	if _, err := scaleGrowth(100, -1); err == nil {
		t.Fatal("negative modifier accepted")
	}
	if value, err := scaleGrowth(100, 10001); err != nil || value != 1100 {
		t.Fatal("configured modifier was limited by an arbitrary gameplay cap")
	}
	if _, err := scaleGrowth(math.MaxInt64, 1); err == nil {
		t.Fatal("overflow was not rejected")
	}
}

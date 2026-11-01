package configcompiler

import (
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"math"
	"testing"
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

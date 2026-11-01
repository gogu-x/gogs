package growth

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"math"
	"testing"
)

func TestOverflowCannotWrapIntoAValidBattleModifier(t *testing.T) {
	value := &pb_common.GrowthBonuses{AttackPermille: math.MaxInt32}
	Add(value, &pb_common.GrowthBonuses{AttackPermille: 1})
	Add(value, &pb_common.GrowthBonuses{AttackPermille: 100})
	if value.GetAttackPermille() >= 0 {
		t.Fatal("invalid configured totals wrapped into a valid modifier")
	}
}

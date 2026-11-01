package growth

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"math"
)

// Add merges additive modifiers for matching battle attributes.
func Add(target, extra *pb_common.GrowthBonuses) {
	if target == nil || extra == nil {
		return
	}
	target.MaxHpPermille = addChecked(target.GetMaxHpPermille(), extra.GetMaxHpPermille())
	target.AttackPermille = addChecked(target.GetAttackPermille(), extra.GetAttackPermille())
	target.DefensePermille = addChecked(target.GetDefensePermille(), extra.GetDefensePermille())
	target.SpeedPermille = addChecked(target.GetSpeedPermille(), extra.GetSpeedPermille())
}

func addChecked(left, right int32) int32 {
	value := int64(left) + int64(right)
	if left < 0 || right < 0 || value > math.MaxInt32 {
		return -1
	}
	return int32(value)
}

// Attribute converts supported protocol attributes into battle modifiers.
func Attribute(attribute, value int32) *pb_common.GrowthBonuses {
	result := &pb_common.GrowthBonuses{}
	switch attribute {
	case 1:
		result.MaxHpPermille = value
	case 2:
		result.AttackPermille = value
	case 3:
		result.DefensePermille = value
	case 4:
		result.SpeedPermille = value
	}
	return result
}

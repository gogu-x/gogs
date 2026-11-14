package loadout

import (
	"github.com/gogu-x/gogs/game/play/internal/player/growth"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

// BattleBonuses 汇总服务端魂玉和天赋加成，不修改模块数据。
func BattleBonuses(jade *souljade.Mgr, talents *talent.Mgr) *pb_common.GrowthBonuses {
	result := souljade.Bonuses(jade)
	growth.Add(result, talent.Bonuses(talents))
	return result
}

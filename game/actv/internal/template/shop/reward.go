package shop

import (
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/ipb"
	"math"
)

// goodsRewards 读取奖励组并按份数展开为稳定快照。
func goodsRewards(group int32, count int64) ([]*ipb.ActvAssetReward, bool) {
	if count <= 0 || count > 100 {
		return nil, false
	}
	var result []*ipb.ActvAssetReward
	var equipment int64
	for _, r := range glconf.GetAllActvRewardCfg() {
		if r.GetGroupID() != group {
			continue
		}
		n := r.GetCount()
		if n <= 0 || n > math.MaxInt64/count {
			return nil, false
		}
		switch r.GetRewardType() {
		case 1:
			if r.GetRewardID() <= 0 {
				return nil, false
			}
		case 2:
			if glconf.GetBattleEquipCfgNoErr(r.GetRewardID()) == nil {
				return nil, false
			}
			equipment += n * count
			if equipment > 10000 {
				return nil, false
			}
		default:
			return nil, false
		}
		result = append(result, &ipb.ActvAssetReward{Type: r.GetRewardType(), ID: r.GetRewardID(), Count: n * count})
	}
	return result, len(result) > 0
}

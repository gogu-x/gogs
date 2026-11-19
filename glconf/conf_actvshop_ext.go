package glconf

import (
	"maps"
	"sync/atomic"
)

type shopGoodsCatalog map[int32]map[int32]*ActvShopGoodsCfg

var shopGoodsGroups atomic.Pointer[shopGoodsCatalog]

// rebuildShopGoodsGroups 在商品或奖励组变化后，按商品组生成已校验的目录。
func rebuildShopGoodsGroups(goods *CsvConf) {
	rewardGroups, _ := shopRewardGroups.Load().(shopRewardIndex)
	if goods == nil || rewardGroups == nil {
		return
	}
	catalog := make(shopGoodsCatalog)
	for position := range goods.NumRecord() {
		row := goods.Record(position).(*ActvShopGoodsCfg)
		group, id := row.GetGroupID(), row.GetCfgID()
		if entries, exists := catalog[group]; exists && entries == nil {
			continue
		}
		_, validRewards := rewardGroups[row.GetRewardGroupID()]
		reset := row.GetLimitResetType()
		if id <= 0 || row.GetCostItemID() <= 0 || row.GetCostCount() <= 0 ||
			row.GetLimit() < 0 || reset < 0 || reset > 1 || !validRewards {
			catalog[group] = nil
			continue
		}
		if catalog[group] == nil {
			catalog[group] = make(map[int32]*ActvShopGoodsCfg)
		}
		catalog[group][id] = row
	}
	shopGoodsGroups.Store(&catalog)
}

// ShopGoods 返回指定活动的已校验商品目录副本。
func ShopGoods(activityID int32) (map[int32]*ActvShopGoodsCfg, bool) {
	row := GetActvOnlineCfgNoErr(activityID)
	if row == nil {
		return nil, false
	}
	config := GetActvShopCfgNoErr(row.GetContentID())
	if config == nil || config.GetGoodsGroupID() <= 0 {
		return nil, false
	}
	catalog := shopGoodsGroups.Load()
	if catalog == nil {
		return nil, false
	}
	goods := (*catalog)[config.GetGoodsGroupID()]
	return maps.Clone(goods), len(goods) > 0
}

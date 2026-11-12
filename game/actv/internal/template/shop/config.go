package shop

import "github.com/gogu-x/gogs/glconf"

// loadGoods 按活动内容和商品组加载商店配置，全部校验通过后才启用商店。
func loadGoods(id int32) (map[int32]*glconf.ActvShopGoodsCfg, bool) {
	row := glconf.GetActvOnlineCfgNoErr(id)
	if row == nil {
		return nil, false
	}
	cfg := glconf.GetActvShopCfgNoErr(row.GetContentID())
	if cfg == nil || cfg.GetGoodsGroupID() <= 0 {
		return nil, false
	}
	goods := make(map[int32]*glconf.ActvShopGoodsCfg)
	for _, g := range glconf.GetAllActvShopGoodsCfg() {
		if g.GetGroupID() != cfg.GetGoodsGroupID() {
			continue
		}
		id := g.GetCfgID()
		if id <= 0 || g.GetCostItemID() <= 0 || g.GetCostCount() <= 0 || g.GetLimit() < 0 || g.GetLimitResetType() < 0 || g.GetLimitResetType() > 1 {
			return nil, false
		}
		if _, ok := goodsRewards(g.GetRewardGroupID(), 1); !ok {
			return nil, false
		}
		goods[id] = g
	}
	return goods, len(goods) > 0
}

package glconf

import (
	"math"
	"sync/atomic"

	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree/tlog"
)

const (
	// MaxShopPurchaseCount 是单次允许购买的最大份数。
	MaxShopPurchaseCount int64 = 100

	// maxShopEquipmentCount 是单次购买允许发放的最大装备数。
	maxShopEquipmentCount int64 = 10000
)

type shopRewardGroup struct {
	rewards  []*ipb.ActvAssetReward
	maxCount int64
}

func (group shopRewardGroup) getRewards() []*ipb.ActvAssetReward { return group.rewards }
func (group shopRewardGroup) getMaxCount() int64                 { return group.maxCount }

type shopRewardIndex map[int32]shopRewardGroup

var shopRewardGroups atomic.Value

// onShopRewardConfigLoaded 使用本次奖励表和已发布的装备表更新奖励组。
func onShopRewardConfigLoaded(rewards *CsvConf) {
	equipment, _ := csvBattleEquipCfg.Load().(*CsvConf)
	rebuildShopRewardGroups(rewards, equipment)
}

// onShopEquipmentConfigLoaded 使用本次装备表和已发布的奖励表更新奖励组。
func onShopEquipmentConfigLoaded(equipment *CsvConf) {
	rewards, _ := csvActvRewardCfg.Load().(*CsvConf)
	rebuildShopRewardGroups(rewards, equipment)
}

// rebuildShopRewardGroups 等两张配置表发布后，校验奖励并整体替换商店奖励组索引。
func rebuildShopRewardGroups(rewards, equipment *CsvConf) {
	index := buildShopRewardIndex(rewards, equipment)
	if index == nil {
		return
	}
	shopRewardGroups.Store(index)
	goods, _ := csvActvShopGoodsCfg.Load().(*CsvConf)
	rebuildShopGoodsGroups(goods)
	tlog.Info("[glconf/rebuildShopRewardGroups] 商店奖励组索引已更新, groups=%v", len(index))
}

func buildShopRewardIndex(rewards, equipment *CsvConf) shopRewardIndex {
	if rewards == nil || equipment == nil {
		return nil
	}

	rowsByGroup := make(map[int32][]*ActvRewardCfg)
	for position := range rewards.NumRecord() {
		row := rewards.Record(position).(*ActvRewardCfg)
		id := row.GetGroupID()
		rowsByGroup[id] = append(rowsByGroup[id], row)
	}

	index := make(shopRewardIndex, len(rowsByGroup))
	for id, groupRows := range rowsByGroup {
		group, ok := buildShopRewardGroup(groupRows, equipment)
		if !ok {
			tlog.Error("[glconf/buildShopRewardIndex] 商店奖励组配置无效, group=%v", id)
			continue
		}
		index[id] = group
	}
	return index
}

// buildShopRewardGroup 校验整组配置并预计算单次可购买的份数。
func buildShopRewardGroup(rows []*ActvRewardCfg, equipment *CsvConf) (shopRewardGroup, bool) {
	rewards := make([]*ipb.ActvAssetReward, 0, len(rows))
	maxCount := MaxShopPurchaseCount
	var equipmentCount int64
	for _, row := range rows {
		rewardType, id, count := row.GetRewardType(), row.GetRewardID(), row.GetCount()
		if count <= 0 {
			return shopRewardGroup{}, false
		}

		switch rewardType {
		case 1:
			if id <= 0 {
				return shopRewardGroup{}, false
			}
		case 2:
			if equipment.GetNoErr(id) == nil || count > maxShopEquipmentCount-equipmentCount {
				return shopRewardGroup{}, false
			}
			equipmentCount += count
		default:
			return shopRewardGroup{}, false
		}

		maxCount = min(maxCount, math.MaxInt64/count)
		rewards = append(rewards, &ipb.ActvAssetReward{Type: rewardType, ID: id, Count: count})
	}
	if equipmentCount > 0 {
		maxCount = min(maxCount, maxShopEquipmentCount/equipmentCount)
	}
	return shopRewardGroup{rewards: rewards, maxCount: maxCount}, true
}

// HasShopRewardGroup 判断商店奖励组是否已通过配置校验。
func HasShopRewardGroup(id int32) bool {
	_, ok := getShopRewardGroup(id)
	return ok
}

// ShopRewards 从已校验的奖励组生成一笔购买的独立奖励快照。
func ShopRewards(id int32, count int64) ([]*ipb.ActvAssetReward, bool) {
	group, ok := getShopRewardGroup(id)
	if !ok || count <= 0 || count > group.getMaxCount() {
		return nil, false
	}
	rewards := group.getRewards()
	snapshot := make([]*ipb.ActvAssetReward, 0, len(rewards))
	for _, reward := range rewards {
		snapshot = append(snapshot, &ipb.ActvAssetReward{
			Type:  reward.GetType(),
			ID:    reward.GetID(),
			Count: reward.GetCount() * count,
		})
	}
	return snapshot, true
}

func getShopRewardGroup(id int32) (shopRewardGroup, bool) {
	index, _ := shopRewardGroups.Load().(shopRewardIndex)
	group, ok := index[id]
	return group, ok
}

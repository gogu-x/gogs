package service

import "github.com/gogu-x/gogs/game/actv/internal/trigger"

// SettlementRetentionSeconds 是活动结束后的结算保留期，单位为秒。
const SettlementRetentionSeconds = trigger.SettlementRetentionSeconds

// IsExpired 判断是否已到结束后十四天；未设置结束时间的活动不自动过期。
func (meta *ActvMeta) IsExpired(now int64) bool {
	end := meta.GetEndTs()
	return end > 0 && now >= end && now-end >= SettlementRetentionSeconds
}

// SharedActivityIDs 返回玩家相关且未过期的共享活动编号，限定玩家组件的加载范围。
func (as *ActvService) SharedActivityIDs(guildID OwnerID, now int64) []int64 {
	var ids []int64
	as.rangePlayerShared(guildID, func(actv *Actv) {
		if !actv.GetMeta().IsExpired(now) {
			ids = append(ids, actv.GetMeta().GetActvID())
		}
	})
	return ids
}

// RemoveExpiredActivities 批量移除已过结算期且无脏数据的实例。
// 脏数据继续由定时存盘处理，成功后下一轮清理，避免丢弃尚未落库的变更。
func (as *ActvService) RemoveExpiredActivities(now int64) {
	var ids []int64
	for _, actv := range as.ListAllActvs() {
		if actv.GetMeta().IsExpired(now) && !actv.hasDirtyData() {
			ids = append(ids, actv.GetMeta().GetActvID())
		}
	}
	as.RemoveActvs(ids)
}

// hasDirtyData 判断实例元数据或任意组件是否仍有待保存的变更。
func (actv *Actv) hasDirtyData() bool {
	if actv.GetMeta().IsDirty() {
		return true
	}
	for _, comp := range actv.comps {
		if comp.IsDirty() {
			return true
		}
	}
	for _, comp := range actv.playerComps {
		if len(comp.DirtyPlayers()) > 0 {
			return true
		}
	}
	return false
}

package service

import (
	"fmt"
	"time"

	"github.com/gogu-x/tree/tlog"
)

// bindComponents 收集模板注册的组件、校验组件编号唯一，并为玩家列表组件绑定新建条目判断。
func (actv *Actv) bindComponents(canCreate func(uid uint64) bool) error {
	actv.comps = make(map[int32]Component)
	actv.playerComps = make(map[int32]PlayerComponent)
	holder, ok := actv.Ext.(ComponentHolder)
	if !ok {
		return nil
	}
	for _, comp := range holder.Components() {
		if comp == nil {
			continue
		}
		if err := actv.checkCompID(comp.CompID()); err != nil {
			return err
		}
		actv.comps[comp.CompID()] = comp
	}
	for _, comp := range holder.PlayerComponents() {
		if comp == nil {
			continue
		}
		if err := actv.checkCompID(comp.CompID()); err != nil {
			return err
		}
		comp.BindCreateChecker(canCreate)
		actv.playerComps[comp.CompID()] = comp
	}
	return nil
}

func (actv *Actv) checkCompID(compID int32) error {
	if compID <= 0 {
		return fmt.Errorf("component id must be positive, comp=%d", compID)
	}
	if _, exists := actv.comps[compID]; exists {
		return fmt.Errorf("duplicate component id, comp=%d", compID)
	}
	if _, exists := actv.playerComps[compID]; exists {
		return fmt.Errorf("duplicate component id, comp=%d", compID)
	}
	return nil
}

// LoadData 将一条组件存盘数据交给对应组件恢复。
// 单条数据组件要求归属者与活动实例一致；玩家列表组件要求归属者为玩家。
func (actv *Actv) LoadData(doc *ActvDataDoc) error {
	if actv == nil || doc == nil {
		return fmt.Errorf("activity data is nil")
	}
	meta := actv.GetMeta()
	if comp := actv.comps[doc.Comp]; comp != nil {
		if doc.OwnerScope != meta.GetOwnerScope() || doc.OwnerID != meta.GetOwnerID() {
			return fmt.Errorf("component data owner mismatch, comp=%d owner=%d/%d expect=%d/%d",
				doc.Comp, doc.OwnerScope, doc.OwnerID, meta.GetOwnerScope(), meta.GetOwnerID())
		}
		return comp.Load(doc.Data)
	}
	if comp := actv.playerComps[doc.Comp]; comp != nil {
		if doc.OwnerScope != ScopePlayer || doc.OwnerID <= 0 {
			return fmt.Errorf("player component data requires player owner, comp=%d owner=%d/%d",
				doc.Comp, doc.OwnerScope, doc.OwnerID)
		}
		return comp.Load(uint64(doc.OwnerID), doc.Data)
	}
	return fmt.Errorf("component not registered, comp=%d", doc.Comp)
}

// appendDirtyComps 追加单条数据组件的脏数据快照，归属者与活动实例一致。
func (actv *Actv) appendDirtyComps(docs []*ActvDataDoc) []*ActvDataDoc {
	meta := actv.GetMeta()
	activityID := meta.GetActvID()
	for compID, comp := range actv.comps {
		if !comp.IsDirty() {
			continue
		}
		raw, err := comp.Save()
		if err != nil {
			tlog.Log.Error("[activity/Actv.appendDirtyComps] 组件数据编码失败, activity=%v comp=%v err=%v", activityID, compID, err)
			continue
		}
		docs = append(docs, newDataSnapshot(activityID, compID, meta.GetOwnerScope(), meta.GetOwnerID(), raw, comp.Revision()))
	}
	return docs
}

// appendDirtyPlayer 追加指定玩家在玩家列表组件中的脏数据快照，归属者为该玩家。
func (actv *Actv) appendDirtyPlayer(docs []*ActvDataDoc, comp PlayerComponent, uid uint64) []*ActvDataDoc {
	activityID := actv.GetMeta().GetActvID()
	raw, err := comp.Save(uid)
	if err != nil {
		tlog.Log.Error("[activity/Actv.appendDirtyPlayer] 玩家组件数据编码失败, activity=%v comp=%v pid=%v err=%v",
			activityID, comp.CompID(), uid, err)
		return docs
	}
	return append(docs, newDataSnapshot(activityID, comp.CompID(), ScopePlayer, OwnerID(uid), raw, comp.Revision(uid)))
}

// DirtyData 返回全部已加载活动组件的脏数据快照。
func (as *ActvService) DirtyData() []*ActvDataDoc {
	var docs []*ActvDataDoc
	for _, actv := range as.ListAllActvs() {
		docs = actv.appendDirtyComps(docs)
		for _, comp := range actv.playerComps {
			for _, uid := range comp.DirtyPlayers() {
				docs = actv.appendDirtyPlayer(docs, comp, uid)
			}
		}
	}
	return docs
}

// PlayerDirtyData 返回随指定玩家加载的全部脏数据快照：
// 玩家独有活动的单条数据，以及相关共享活动玩家列表中该玩家的条目。
func (as *ActvService) PlayerDirtyData(uid uint64, guildID OwnerID) []*ActvDataDoc {
	var docs []*ActvDataDoc
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		docs = actv.appendDirtyComps(docs)
	}
	as.rangePlayerShared(guildID, func(actv *Actv) {
		for _, comp := range actv.playerComps {
			if comp.IsDirty(uid) {
				docs = actv.appendDirtyPlayer(docs, comp, uid)
			}
		}
	})
	return docs
}

// PlayerDirtyMetas 返回指定玩家独有活动中需要存盘的元数据。
func (as *ActvService) PlayerDirtyMetas(uid uint64) []*ActvMeta {
	var metas []*ActvMeta
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		if meta := actv.GetMeta(); meta.IsDirty() {
			metas = append(metas, meta)
		}
	}
	return metas
}

// ClearDataDirty 在存盘完成后清除对应组件的脏标记；数据已卸载时忽略。
func (as *ActvService) ClearDataDirty(doc *ActvDataDoc) {
	if doc == nil {
		return
	}
	actv := as.GetActvByID(doc.ActvID)
	if actv == nil {
		return
	}
	if comp := actv.comps[doc.Comp]; comp != nil {
		comp.ClearDirty(doc.revision)
		return
	}
	if comp := actv.playerComps[doc.Comp]; comp != nil {
		comp.ClearDirty(uint64(doc.OwnerID), doc.revision)
	}
}

// SetPlayerLoaded 标记玩家活动数据已加载。这是“玩家是否已加载”的唯一数据来源：
// 玩家列表组件通过绑定的判断函数实时查询，进行中的活动（含之后新增的）即可为该玩家新建条目。
func (as *ActvService) SetPlayerLoaded(uid uint64) {
	as.loadedPlayers[uid] = true
}

// canCreatePlayerEntry 返回活动的玩家列表组件能否为玩家新建条目：玩家已加载且活动未结束。
func (as *ActvService) canCreatePlayerEntry(actv *Actv) func(uid uint64) bool {
	return func(uid uint64) bool {
		return as.loadedPlayers[uid] && !actv.IsEnded() && !actv.GetMeta().IsExpired(time.Now().Unix())
	}
}

// IsPlayerLoaded 返回玩家活动数据是否已加载。
func (as *ActvService) IsPlayerLoaded(uid uint64) bool { return as.loadedPlayers[uid] }

// NotifyPlayerLoaded 通知玩家相关的进行中共享活动（本服活动 + 所属联盟活动）：该玩家的组件数据已恢复。
// 玩家独有活动通过 OnLoaded 获知数据恢复，不重复通知。
func (as *ActvService) NotifyPlayerLoaded(uid uint64, guildID OwnerID) {
	now := time.Now().Unix()
	as.rangePlayerShared(guildID, func(actv *Actv) {
		if !actv.IsEnded() && !actv.GetMeta().IsExpired(now) {
			actv.GetExt().OnPlayerLoaded(uid)
		}
	})
}

// NotifyPlayerUnload 通知玩家相关的进行中共享活动：该玩家即将卸载。
// 调用发生在取存盘快照之前，模板在钩子中的修改会随释放一起存盘。
func (as *ActvService) NotifyPlayerUnload(uid uint64, guildID OwnerID) {
	now := time.Now().Unix()
	as.rangePlayerShared(guildID, func(actv *Actv) {
		if !actv.IsEnded() && !actv.GetMeta().IsExpired(now) {
			actv.GetExt().OnPlayerUnload(uid)
		}
	})
}

// rangePlayerShared 按归属索引遍历与玩家相关的共享活动：本服服务器活动，以及玩家所属联盟的活动。
// guildID 为 0 表示玩家无联盟或联盟未知，只遍历服务器活动。遍历不分配内存。
func (as *ActvService) rangePlayerShared(guildID OwnerID, fn func(actv *Actv)) {
	for _, actv := range as.ListServerActvs() {
		fn(actv)
	}
	if guildID > 0 {
		for _, actv := range as.ListActvs(ScopeGuild, guildID) {
			fn(actv)
		}
	}
}

// isPlayerShared 返回共享活动是否与玩家相关：服务器活动，或玩家所属联盟的活动。
func (as *ActvService) isPlayerShared(actv *Actv, guildID OwnerID) bool {
	meta := actv.GetMeta()
	switch meta.GetOwnerScope() {
	case ScopeServer:
		return meta.GetOwnerID() == as.serverID
	case ScopeGuild:
		return guildID > 0 && meta.GetOwnerID() == guildID
	}
	return false
}

// UnloadPlayer 移除玩家独有的活动实例，并从相关共享活动的玩家列表组件中卸载该玩家条目。
// 调用方需先取走该玩家的脏数据快照。
func (as *ActvService) UnloadPlayer(uid uint64, guildID OwnerID) {
	delete(as.loadedPlayers, uid)
	activities := as.ListActvs(ScopePlayer, OwnerID(uid))
	ids := make([]int64, 0, len(activities))
	for _, actv := range activities {
		ids = append(ids, actv.GetMeta().GetActvID())
	}
	as.RemoveActvs(ids)
	tlog.Log.Info("[activity/ActvService.UnloadPlayer] 卸载玩家活动数据, pid=%v activities=%v", uid, len(ids))
	as.rangePlayerShared(guildID, func(actv *Actv) {
		actv.unloadPlayer(uid)
	})
}

// ApplyDataDocs 将组件存盘数据分发给已恢复的活动实例；实例不存在或数据无效时跳过并记录日志。
// guildID 为玩家所属联盟：玩家列表条目只挂入与该玩家相关的共享活动，其他联盟活动的条目跳过，
// 保证释放时按同一索引能完整卸载。启动加载不含玩家条目，传 0 即可。
func (as *ActvService) ApplyDataDocs(docs []ActvDataDoc, guildID OwnerID) {
	now := time.Now().Unix()
	for index := range docs {
		doc := &docs[index]
		actv := as.GetActvByID(doc.ActvID)
		if actv == nil {
			tlog.Log.Error("[activity/ActvService.ApplyDataDocs] 组件数据所属活动实例未加载，跳过, activity=%v comp=%v owner=%v/%v",
				doc.ActvID, doc.Comp, doc.OwnerScope, doc.OwnerID)
			continue
		}
		if actv.GetMeta().IsExpired(now) {
			continue
		}
		if _, isPlayerComp := actv.playerComps[doc.Comp]; isPlayerComp && !as.isPlayerShared(actv, guildID) {
			tlog.Log.Error("[activity/ActvService.ApplyDataDocs] 玩家条目所属活动与玩家无关，跳过, activity=%v comp=%v owner=%v/%v guild=%v",
				doc.ActvID, doc.Comp, doc.OwnerScope, doc.OwnerID, guildID)
			continue
		}
		if err := actv.LoadData(doc); err != nil {
			tlog.Log.Error("[activity/ActvService.ApplyDataDocs] 组件数据恢复失败，跳过, activity=%v comp=%v owner=%v/%v err=%v",
				doc.ActvID, doc.Comp, doc.OwnerScope, doc.OwnerID, err)
		}
	}
}

// SettleEndedPlayer 处理与玩家相关的已结束共享活动中刚加载的玩家数据：
// 模块实现 Settler 且 NeedSettle 为 true 时调用 Settle；不需要结算的活动直接卸载该玩家条目。
// 返回结算产生的脏数据快照，以及这些快照所属的活动，调用方存盘成功后再调用 UnloadEndedPlayer 卸载。
func (as *ActvService) SettleEndedPlayer(uid uint64, guildID OwnerID) ([]*ActvDataDoc, []int64) {
	var docs []*ActvDataDoc
	var settled []int64
	now := time.Now().Unix()
	as.rangePlayerShared(guildID, func(actv *Actv) {
		if !actv.IsEnded() || actv.GetMeta().IsExpired(now) || len(actv.playerComps) == 0 {
			return
		}
		settler, ok := actv.GetExt().(Settler)
		if !ok || !settler.NeedSettle(uid) {
			actv.unloadPlayer(uid)
			return
		}
		settler.Settle(uid)
		tlog.Log.Info("[activity/ActvService.SettleEndedPlayer] 已结束活动玩家结算, activity=%v pid=%v", actv.GetMeta().GetActvID(), uid)
		before := len(docs)
		for _, comp := range actv.playerComps {
			if comp.IsDirty(uid) {
				docs = actv.appendDirtyPlayer(docs, comp, uid)
			}
		}
		if len(docs) == before {
			actv.unloadPlayer(uid) // 结算未修改数据，无需存盘。
			return
		}
		settled = append(settled, actv.GetMeta().GetActvID())
	})
	return docs, settled
}

// UnloadEndedPlayer 结算存盘完成后，从指定已结束活动中卸载玩家条目；仍为脏数据的组件保留，
// 由定时存盘或玩家释放继续处理。
func (as *ActvService) UnloadEndedPlayer(uid uint64, activityIDs []int64) {
	for _, activityID := range activityIDs {
		actv := as.GetActvByID(activityID)
		if actv == nil || !actv.IsEnded() {
			continue
		}
		for _, comp := range actv.playerComps {
			if !comp.IsDirty(uid) {
				comp.Unload(uid)
			}
		}
	}
}

func (actv *Actv) unloadPlayer(uid uint64) {
	for _, comp := range actv.playerComps {
		comp.Unload(uid)
	}
}

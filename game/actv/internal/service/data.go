package service

import (
	"fmt"

	"github.com/gogu-x/tree/tlog"
)

// bindComponents 收集模板注册的组件并校验组件编号唯一。
func (actv *Actv) bindComponents() error {
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
// 玩家独有活动的单条数据，以及所有活动玩家列表中该玩家的条目。
func (as *ActvService) PlayerDirtyData(uid uint64) []*ActvDataDoc {
	var docs []*ActvDataDoc
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		docs = actv.appendDirtyComps(docs)
	}
	for _, actv := range as.ListAllActvs() {
		for _, comp := range actv.playerComps {
			if comp.IsDirty(uid) {
				docs = actv.appendDirtyPlayer(docs, comp, uid)
			}
		}
	}
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

// SetPlayerLoaded 标记玩家活动数据已加载：所有活动的玩家列表组件允许为其新建条目，
// 之后新增的服务器/联盟活动也会自动补标记。
func (as *ActvService) SetPlayerLoaded(uid uint64) {
	as.loadedPlayers[uid] = true
	for _, actv := range as.ListAllActvs() {
		for _, comp := range actv.playerComps {
			comp.SetLoaded(uid)
		}
	}
}

// IsPlayerLoaded 返回玩家活动数据是否已加载。
func (as *ActvService) IsPlayerLoaded(uid uint64) bool { return as.loadedPlayers[uid] }

// NotifyPlayerLoaded 通知所有服务器/联盟活动：该玩家的组件数据已恢复到实例中。
// 玩家独有活动通过 OnLoaded 获知数据恢复，不重复通知。
func (as *ActvService) NotifyPlayerLoaded(uid uint64) {
	for _, actv := range as.listSharedActvs() {
		actv.GetExt().OnPlayerLoaded(uid)
	}
}

// NotifyPlayerUnload 通知所有服务器/联盟活动：该玩家即将卸载。
// 调用发生在取存盘快照之前，模板在钩子中的修改会随释放一起存盘。
func (as *ActvService) NotifyPlayerUnload(uid uint64) {
	for _, actv := range as.listSharedActvs() {
		actv.GetExt().OnPlayerUnload(uid)
	}
}

// listSharedActvs 返回服务器/联盟归属的活动实例。
func (as *ActvService) listSharedActvs() []*Actv {
	all := as.ListAllActvs()
	shared := make([]*Actv, 0, len(all))
	for _, actv := range all {
		if actv.GetMeta().GetOwnerScope() != ScopePlayer && actv.GetExt() != nil {
			shared = append(shared, actv)
		}
	}
	return shared
}

// UnloadPlayer 移除玩家独有的活动实例，并从所有活动玩家列表组件中卸载该玩家条目。
// 调用方需先取走该玩家的脏数据快照。
func (as *ActvService) UnloadPlayer(uid uint64) {
	delete(as.loadedPlayers, uid)
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		as.RemoveActv(actv.GetMeta().GetActvID())
	}
	for _, actv := range as.ListAllActvs() {
		for _, comp := range actv.playerComps {
			comp.Unload(uid)
		}
	}
}

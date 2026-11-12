package service

import (
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
	"strconv"
)

// CreatePlayerActvs 按配置创建缺少的个人活动；完成实例也参与去重。
// 历史元数据即使无法恢复行为，也参与去重，避免覆盖或重建一次性活动。
func (as *ActvService) CreatePlayerActvs(
	uid uint64, rows []*glconf.ActvOnlineCfg, history map[int32]bool, now int64,
) []*Actv {
	if uid == 0 || uid > uint64(1<<63-1) || !as.IsPlayerLoaded(uid) {
		return nil
	}
	existing := make(map[int32]bool)
	for cfgID := range history {
		existing[cfgID] = true
	}
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		existing[actv.GetMeta().GetCfgID()] = true
	}
	var created []*Actv
	for _, row := range trigger.PlayerCreationConfigs(rows, existing) {
		cfgID := row.GetID()
		scope, ok := as.TplScope(TemplateType(row.GetType()))
		if !ok || scope != ScopePlayer {
			continue
		}
		if as.playerIDs == nil {
			as.playerIDs, _ = comm.NewIDGenerator(int64(as.serverID) & 1023)
		}
		meta := &ActvMeta{
			ActvID: as.playerIDs.NextID(), CfgID: cfgID, OwnerID: OwnerID(uid),
			OwnerScope: ScopePlayer, TemplateType: TemplateType(row.GetType()), StartTs: now,
		}
		if duration, err := trigger.ParseDuration(row.GetDuration()); err != nil || duration < 0 {
			continue
		} else if duration > 0 {
			meta.EndTs = now + duration
		}
		if row.GetTriggerType() == 5 {
			parentCfg, err := strconv.ParseInt(row.GetTriggerVal(), 10, 32)
			if err != nil {
				continue
			}
			var parent *ActvMeta
			for _, a := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
				if a.GetMeta().GetCfgID() == int32(parentCfg) {
					parent = a.GetMeta()
					break
				}
			}
			shop := glconf.GetActvShopCfgNoErr(row.GetContentID())
			if parent == nil || shop == nil || shop.GetCloseDelay() < 0 || shop.GetCloseDelay() > SettlementRetentionSeconds || parent.GetEndTs() <= 0 {
				continue
			}
			meta.ParentActvID = parent.GetActvID()
			meta.StartTs = parent.GetStartTs()
			meta.EndTs = parent.GetEndTs() + shop.GetCloseDelay()
			if now >= meta.EndTs {
				continue
			}
		}
		actv := as.NewActv(meta, NewActvConfig(row, meta))
		if actv == nil || !as.AddActv(actv) {
			continue
		}
		as.Loaded(actv)
		actv.GetExt().OnStart()
		meta.Started = true
		meta.MarkDirty()
		existing[cfgID] = true
		created = append(created, actv)
		tlog.Log.Info(
			"[actv/CreatePlayerActvs] 按配置创建个人活动, pid=%v config=%v actv=%v",
			uid, cfgID, meta.GetActvID(),
		)
	}
	return created
}

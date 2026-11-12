package service

import (
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
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
	for _, row := range trigger.PlayerOnceConfigs(rows, existing) {
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
		actv := as.NewActv(meta, NewActvConfig(row, meta))
		if actv == nil || !as.AddActv(actv) {
			continue
		}
		actv.GetExt().OnLoaded()
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

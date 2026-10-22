package service

import (
	"fmt"

	activityconf "github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/tlog"
)

// NewActivityConfig 由配置行和实例元数据组装模板使用的运行时配置。
func NewActivityConfig(row *glconf.ActvOnlineCfg, meta *ActvMeta) activityconf.ActivityConfig {
	return activityconf.ActivityConfig{
		ID:        uint64(row.GetID()),
		Name:      fmt.Sprintf("%d", row.GetContentID()),
		Type:      int32(meta.GetTemplateType()),
		ConfigRow: row,
		Scope:     int32(meta.GetOwnerScope()),
		StartTime: meta.GetStartTs(),
		EndTime:   meta.GetEndTs(),
	}
}

// FilterServerPlans 保留模板已注册且声明为服务器归属的时间规则；其余按原因汇总记录日志。
// 联盟、玩家归属的活动需要按联盟/玩家创建，当前版本暂不由定时器创建。
func (as *ActvService) FilterServerPlans(plans []*trigger.Plan) []*trigger.Plan {
	result := make([]*trigger.Plan, 0, len(plans))
	unregistered := make(map[TemplateType]int)
	otherScope := make(map[OwnerScope]int)
	for _, plan := range plans {
		scope, ok := as.TplScope(TemplateType(plan.TemplateType))
		switch {
		case !ok:
			unregistered[TemplateType(plan.TemplateType)]++
		case scope != ScopeServer:
			otherScope[scope]++
		default:
			result = append(result, plan)
		}
	}
	for templateType, count := range unregistered {
		tlog.Log.Info("[activity/ActvService.FilterServerPlans] 活动模板未注册，跳过, template=%v count=%v", templateType, count)
	}
	for scope, count := range otherScope {
		tlog.Log.Info("[activity/ActvService.FilterServerPlans] 非服务器归属活动暂不由定时器创建，跳过, scope=%v count=%v", scope, count)
	}
	return result
}

// CreateDueServerActvs 按时间规则创建 now 时刻处于有效期内、尚未存在的服务器活动实例。
// 实例编号由配置和轮次确定，已存在（含启动时从库中恢复）的实例不会重复创建。返回新创建的实例。
func (as *ActvService) CreateDueServerActvs(plans []*trigger.Plan, now int64) []*Actv {
	var created []*Actv
	for _, plan := range plans {
		for _, round := range plan.ActiveRounds(now) {
			activityID := plan.InstanceID(round.Round)
			if as.GetActvByID(activityID) != nil {
				continue
			}
			meta := &ActvMeta{
				ActvID:       activityID,
				StartTs:      round.StartTs,
				EndTs:        round.EndTs,
				CfgID:        plan.CfgID,
				OwnerID:      as.serverID,
				OwnerScope:   ScopeServer,
				TemplateType: TemplateType(plan.TemplateType),
			}
			actv := as.NewActv(meta, NewActivityConfig(plan.Cfg, meta))
			if actv == nil || !as.AddActv(actv) {
				tlog.Log.Error("[activity/ActvService.CreateDueServerActvs] 活动实例创建失败, activity=%v config=%v round=%v",
					activityID, plan.CfgID, round.Round)
				continue
			}
			actv.GetExt().OnLoaded() // 新实例没有存盘数据，组件为初始值。
			tlog.Log.Info("[activity/ActvService.CreateDueServerActvs] 创建活动实例, activity=%v config=%v round=%v start=%v end=%v",
				activityID, plan.CfgID, round.Round, round.StartTs, round.EndTs)
			created = append(created, actv)
		}
	}
	return created
}

// EndDueActivities 结束已到结束时间的进行中活动。
func (as *ActvService) EndDueActivities(now int64) {
	for _, actv := range as.orderedActvs {
		meta := actv.GetMeta()
		if actv.IsEnded() || meta.GetEndTs() <= 0 || now < meta.GetEndTs() {
			continue
		}
		as.EndActv(meta.GetActvID())
	}
}

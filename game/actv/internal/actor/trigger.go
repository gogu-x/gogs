package actor

import (
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// actvTriggerInterval 表示按配置触发创建活动实例的检查间隔。
	actvTriggerInterval  = time.Minute
	actvTriggerTimerType = timer.TimerType(1000014)
)

// registerTriggerTimer 注册触发创建定时器的回调。
func (actor *Actv) registerTriggerTimer() {
	actor.wheel.Register(actvTriggerTimerType, actor.onTriggerTimer)
}

// startTrigger 在活动数据加载完成且收到开服事件后启动：解析配置时间规则，立即检查一次，之后每分钟检查。
// 只启动一次；加载失败时不启动，避免在缺少已有实例的情况下重复创建。
func (actor *Actv) startTrigger() {
	if actor.triggerStarted || !actor.ready || actor.loadErr != nil || !actor.serverOpened {
		return
	}
	actor.triggerStarted = true
	if conf.ServerOpenTs <= 0 {
		tlog.Log.Error("[actv/Actv.startTrigger] 未配置开服时间 --server-open-ts，开服后 N 天类活动不会触发")
	}
	actor.ensureCreationTrigger()
	actor.runTrigger()
}

// onTriggerTimer 将定时到期回调投递到 Actv Actor 线程执行。
func (actor *Actv) onTriggerTimer(_ interface{}) {
	if actor == nil || actor.selfPID.ID == 0 {
		return
	}
	if !tree.SendCallback(actor.selfPID, func(_ tree.Context, _ interface{}, _ error) {
		actor.runTrigger()
	}, nil, nil) {
		tlog.Log.Error("[actv/Actv.onTriggerTimer] 活动触发回调投递失败")
	}
}

// runTrigger 创建到期的服务器活动实例并安排下一次检查。
func (actor *Actv) runTrigger() {
	if actor.serverClosing {
		return
	}
	actor.ensureCreationTrigger()
	now := time.Now().Unix()
	selected := actor.creationTrigger.Check(trigger.OnServerTick, now, nil)
	created := actor.actvService.CreateDueServerActs(selected.ServerPlans(), now)
	if len(created) > 0 {
		tlog.Log.Info("[actv/Actv.runTrigger] 按配置创建活动实例, count=%v", len(created))
	}
	actor.wheel.After(actvTriggerTimerType, actvTriggerInterval, nil)
}

// ensureCreationTrigger 在统一触发模块解析并缓存配置；玩家初始化与服务器定时器共用。
func (actor *Actv) ensureCreationTrigger() {
	if actor.creationTrigger != nil {
		return
	}
	actor.creationTrigger = trigger.NewEngine(
		glconf.GetAllActvOnlineCfg(), actor.actvTemplateScope, conf.ServerOpenTs, time.Local,
	)
}

// actvTemplateScope 适配模板归属查询，trigger 无需依赖活动服务。
func (actor *Actv) actvTemplateScope(templateType int32) int32 {
	scope, ok := actor.actvService.TplScope(service.TemplateType(templateType))
	if !ok {
		return 0
	}
	return int32(scope)
}

// initializePlayerActvs 玩家初始化只提供创建历史，配置判断统一交给 trigger。
func (actor *Actv) initializePlayerActvs(uid uint64, state *playerLoadState) {
	actor.ensureCreationTrigger()
	history := make(map[int32]bool)
	if state.metas != nil {
		for _, meta := range *state.metas {
			history[meta.GetCfgID()] = true
		}
	}
	for _, actv := range actor.actvService.ListActvs(service.ScopePlayer, service.OwnerID(uid)) {
		history[actv.GetMeta().GetCfgID()] = true
	}
	now := time.Now().Unix()
	selected := actor.creationTrigger.Check(trigger.OnPlayerInit, now, history)
	actor.actvService.CreatePlayerActvs(
		uid, selected.PlayerConfigs(), history, now,
	)
}

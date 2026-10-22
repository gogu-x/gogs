package actor

import (
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/game/actv/internal/trigger"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// activityTriggerInterval 表示按配置触发创建活动实例的检查间隔。
	activityTriggerInterval  = time.Minute
	activityTriggerTimerType = timer.TimerType(1000014)
)

// registerTriggerTimer 注册触发创建定时器的回调。
func (actor *Activity) registerTriggerTimer() {
	actor.wheel.Register(activityTriggerTimerType, func(interface{}) {
		actor.onTriggerTimer()
	})
}

// startTrigger 在活动数据加载完成且收到开服事件后启动：解析配置时间规则，立即检查一次，之后每分钟检查。
// 只启动一次；加载失败时不启动，避免在缺少已有实例的情况下重复创建。
func (actor *Activity) startTrigger() {
	if actor.triggerStarted || !actor.ready || actor.loadErr != nil || !actor.serverOpened {
		return
	}
	actor.triggerStarted = true
	if conf.ServerOpenTs <= 0 {
		tlog.Log.Error("[activity/Activity.startTrigger] 未配置开服时间 --server-open-ts，开服后 N 天类活动不会触发")
	}
	plans := trigger.BuildPlans(glconf.GetAllActvOnlineCfg(), conf.ServerOpenTs, time.Local)
	actor.triggerPlans = actor.actvService.FilterServerPlans(plans)
	tlog.Log.Info("[activity/Activity.startTrigger] 活动触发规则已加载, plans=%v serverPlans=%v", len(plans), len(actor.triggerPlans))
	actor.runTrigger()
}

// onTriggerTimer 将定时到期回调投递到 Activity Actor 线程执行。
func (actor *Activity) onTriggerTimer() {
	if actor == nil || actor.system == nil {
		return
	}
	if !actor.system.SendCallback(actor.selfPID, func(_ tree.Context, _ interface{}, _ error) {
		actor.runTrigger()
	}, nil, nil) {
		tlog.Log.Error("[activity/Activity.onTriggerTimer] 活动触发回调投递失败")
	}
}

// runTrigger 创建到期的服务器活动实例并安排下一次检查。
func (actor *Activity) runTrigger() {
	if actor.serverClosing {
		return
	}
	created := actor.actvService.CreateDueServerActvs(actor.triggerPlans, time.Now().Unix())
	if len(created) > 0 {
		tlog.Log.Info("[activity/Activity.runTrigger] 按配置创建活动实例, count=%v", len(created))
	}
	actor.wheel.After(activityTriggerTimerType, activityTriggerInterval, nil)
}

package actor

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// activityLifecycleTickInterval 表示活动生命周期推进的间隔。
	activityLifecycleTickInterval = time.Second
	activityLifecycleTimerType    = timer.TimerType(1000011)
)

// registerTimers 创建供 Mongo 存盘生命周期使用的时间轮。
func (actor *Activity) registerTimers(ctx tree.Context) {
	actor.system = ctx.System()
	actor.selfPID = ctx.Self()
	actor.wheel = timer.NewTimeWheel(32, actor.selfPID, actor.system)
	actor.wheel.Register(activityLifecycleTimerType, func(interface{}) {
		actor.onLifecycleTimer()
	})
}

// onLifecycleTimer 将定时到期回调一次性投递到所属 Activity Actor，并携带其 Context。
func (actor *Activity) onLifecycleTimer() {
	if actor == nil || actor.system == nil {
		return
	}
	if !actor.system.SendCallback(actor.selfPID, func(ctx tree.Context, _ interface{}, _ error) {
		actor.onLifecycleTick(ctx)
	}, nil, nil) {
		tlog.Log.Error("[activity/Activity.onLifecycleTimer] 活动生命周期回调投递失败")
	}
}

// scheduleLifecycleTick 安排下一次活动生命周期推进。
func (actor *Activity) scheduleLifecycleTick() {
	actor.wheel.After(activityLifecycleTimerType, activityLifecycleTickInterval, nil)
}

// onLifecycleTick 推进已加载归属对象的活动阶段并安排下一轮定时。
func (actor *Activity) onLifecycleTick(ctx tree.Context) {
	if actor == nil || actor.serverClosing {
		return
	}
	if actor.actvService != nil {
		now := time.Now().Unix()
		actor.actvService.StartDueActivities(now)
		actor.actvService.HandleTicker()
	}
	actor.scheduleLifecycleTick()
}

package activity

import (
	"time"

	"github.com/gogu-x/gogs/game/activity/internal"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// activityLifecycleTimerType 表示服务器活动生命周期定时器类型。
	activityLifecycleTimerType = timer.TimerType(1000005)
	// activityLifecycleTickInterval 表示活动生命周期推进的间隔。
	activityLifecycleTickInterval = time.Second
)

// registerTimers 创建活动 Actor 的时间轮并注册定时回调。
func (actor *Activity) registerTimers(ctx tree.Context) {
	actor.system = ctx.System()
	actor.selfPID = ctx.Self()
	actor.wheel = timer.NewTimeWheel(32, actor.selfPID, actor.system)
	actor.wheel.Register(activityLifecycleTimerType, actor.onLifecycleTimer)
}

// onLifecycleTimer 将时间轮回调转换为活动 Actor 内部消息。
func (actor *Activity) onLifecycleTimer(_ interface{}) {
	if actor.system == nil || actor.selfPID.ID == 0 || !actor.system.TrySend(actor.selfPID, &internal.LifecycleTick{}) {
		tlog.Log.Error("[activity/Activity.onLifecycleTimer] 投递活动生命周期定时消息失败")
	}
}

// scheduleLifecycleTick 安排下一次活动生命周期推进。
func (actor *Activity) scheduleLifecycleTick() {
	if actor.wheel == nil {
		return
	}
	if actor.wheel.After(activityLifecycleTimerType, activityLifecycleTickInterval, nil) == nil {
		tlog.Log.Error("[activity/Activity.scheduleLifecycleTick] 安排服务器活动定时器失败")
	}
}

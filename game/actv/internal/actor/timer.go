package actor

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// actvLifecycleTickInterval 表示活动生命周期推进的间隔。
	actvLifecycleTickInterval = time.Second
	actvLifecycleTimerType    = timer.TimerType(1000011)
)

// registerTimers 创建供 Mongo 存盘生命周期使用的时间轮。
func (actor *Actv) registerTimers(ctx tree.Context) {
	actor.system = ctx.System()
	actor.selfPID = ctx.Self()
	actor.wheel = timer.NewTimeWheel(32, actor.selfPID, actor.system)
	actor.wheel.Register(actvLifecycleTimerType, func(interface{}) {
		actor.onLifecycleTimer()
	})
	actor.registerTriggerTimer()
}

// onLifecycleTimer 将定时到期回调一次性投递到所属 Actv Actor，并携带其 Context。
func (actor *Actv) onLifecycleTimer() {
	if actor == nil || actor.system == nil {
		return
	}
	if !actor.system.SendCallback(actor.selfPID, func(ctx tree.Context, _ interface{}, _ error) {
		actor.onLifecycleTick(ctx)
	}, nil, nil) {
		tlog.Log.Error("[actv/Actv.onLifecycleTimer] 活动生命周期回调投递失败")
	}
}

// scheduleLifecycleTick 安排下一次活动生命周期推进。
func (actor *Actv) scheduleLifecycleTick() {
	actor.wheel.After(actvLifecycleTimerType, actvLifecycleTickInterval, nil)
}

// onLifecycleTick 推进活动阶段（开始、结束）并安排下一轮定时。
// 加载完成前不推进，避免模板在组件数据恢复前执行 OnStart；收到开服事件前不开始也不结束活动。
func (actor *Actv) onLifecycleTick(ctx tree.Context) {
	if actor == nil || actor.serverClosing {
		return
	}
	if actor.actvService != nil && actor.ready && actor.loadErr == nil && actor.serverOpened {
		now := time.Now().Unix()
		actor.actvService.StartDueActvs(now)
		actor.actvService.EndDueActvs(now)
		actor.actvService.RemoveExpiredActvs(now)
		actor.actvService.HandleTicker()
	}
	actor.scheduleLifecycleTick()
}

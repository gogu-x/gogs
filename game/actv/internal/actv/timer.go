package actv

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

// actvLifecycleTickInterval 表示活动生命周期推进的间隔。
const actvLifecycleTickInterval = time.Second

// 定时器类型只需在当前活动Actor的时间轮内唯一，统一使用枚举分配。
const (
	actvLifecycleTimerType timer.TimerType = iota + 1 // 活动生命周期推进。
	actvSaveTickerType                                // 活动元数据定时存盘。
	actvDataSaveTickerType                            // 活动组件数据定时存盘。
	actvTriggerTimerType                              // 按配置检查活动创建条件。
)

// registerTimers 创建供 Mongo 存盘生命周期使用的时间轮。
func (actor *Actv) registerTimers(ctx tree.Context) {

	actor.selfPID = ctx.Self()
	actor.wheel = timer.NewTimeWheel(32, actor.selfPID)
	actor.wheel.Register(actvLifecycleTimerType, actor.onLifecycleTimer)
	actor.registerTriggerTimer()
}

// onLifecycleTimer 将定时到期回调一次性投递到所属 Actv Actor，并携带其 Context。
func (actor *Actv) onLifecycleTimer(_ interface{}) {
	if actor == nil || actor.selfPID.ID == 0 {
		return
	}
	if !tree.SendCallback(actor.selfPID, func(ctx tree.Context, _ interface{}, _ error) {
		actor.onLifecycleTick(ctx)
	}, nil, nil) {
		tlog.Log.Error("[actv/Actv.onLifecycleTimer] 活动生命周期回调投递失败")
	}
}

// scheduleLifecycleTick 安排下一次活动生命周期推进。
func (actor *Actv) scheduleLifecycleTick() {
	actor.wheel.After(actvLifecycleTimerType, actvLifecycleTickInterval, nil)
}

// onLifecycleTick 推进活动开始、结束及模板每日刷新，并安排下一轮定时。
// 加载完成前不推进，避免模板在组件数据恢复前执行 OnStart；收到开服事件前不开始也不结束活动。
func (actor *Actv) onLifecycleTick(ctx tree.Context) {
	if actor == nil || actor.serverClosing {
		return
	}
	if actor.actvService != nil && actor.ready && actor.loadErr == nil && actor.serverOpened {
		now := comm.NowUnix()
		actor.actvService.StartDueActvs(now)
		actor.actvService.EndDueActvs(now)
		actor.actvService.RemoveExpiredActvs(now)
		actor.actvService.HandleTicker()
	}
	actor.scheduleLifecycleTick()
}

// registerTriggerTimer 注册触发创建定时器的回调。
func (actor *Actv) registerTriggerTimer() {
	actor.wheel.Register(actvTriggerTimerType, actor.onTriggerTimer)
}

package internal

import (
	"github.com/gogu-x/tree"
	"time"

	"github.com/gogu-x/tree/timer"
)

const (
	// resultRetryTimer 仅负责结束通知重试，不参与战斗逻辑时间推进。
	resultRetryTimer = timer.TimerType(1)
	// timeWheelChanLen 是结果重试时间轮的队列容量。
	timeWheelChanLen = 16
)

// initTimer 注册结果投递定时任务。
func (a *BattleActor) initTimer() {
	a.timer = timer.NewTimeWheel(timeWheelChanLen, a.self)
	a.timer.Register(resultRetryTimer, a.onRetryTimer)
}

// onRetryTimer 将重试交给战斗 Actor 串行处理。
func (a *BattleActor) onRetryTimer(interface{}) {
	tree.Send(a.self, &retryTick{})
}

// scheduleRetry 安排确认超时检查，次数耗尽后由 retry 回收 Actor。
func (a *BattleActor) scheduleRetry() {
	delay := a.p.RetryDelay
	if delay <= 0 {
		delay = time.Second
	}
	a.timer.After(resultRetryTimer, delay, nil)
}

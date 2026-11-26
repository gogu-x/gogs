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

// initTimer 注册结果重试和状态唤醒回调，具体触发时间由各自的调度函数设置。
func (a *BattleActor) initTimer() {
	a.timer = timer.NewTimeWheel(timeWheelChanLen, a.self)
	a.timer.Register(resultRetryTimer, a.onRetryTimer)
	a.timer.Register(liveStatusTimer, a.onLiveStatus)
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

// liveStatusTimer 仅处理持续伤害、治疗及状态到期。
const liveStatusTimer = timer.TimerType(2)

// statusWake 通知 BattleActor 处理当前已到期的状态，多个状态可以共用一次唤醒。
type statusWake struct{}

// scheduleStatus 按引擎队首状态事件的毫秒时间安排下一次唤醒。
func (a *BattleActor) scheduleStatus() {
	next, ok := a.p.Battle.NextDueMS()
	if !ok || next == a.live.statusDueMS {
		return
	}
	// 队首时间有变，例如新施加的中毒会先于护盾到期。替换旧唤醒，状态事件仍由引擎保存。
	a.timer.Cancel(liveStatusTimer)
	a.live.statusDueMS = next
	// 从开战时刻换算绝对到期时间，避免每次重新调度时把已过去的时间再等一遍。
	delay := time.Until(a.live.start.Add(time.Duration(next) * time.Millisecond))
	a.timer.After(liveStatusTimer, max(time.Millisecond, delay), nil)
}

// onLiveStatus 唤醒战斗实例处理状态，邮箱拥塞时重试。
func (a *BattleActor) onLiveStatus(interface{}) {
	if !tree.TrySend(a.self, &statusWake{}) {
		a.timer.After(liveStatusTimer, time.Millisecond, nil)
	}
}

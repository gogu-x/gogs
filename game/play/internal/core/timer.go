package core

import (
	"time"

	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	playerAutoSaveTimerType = timer.TimerType(1000009)
	playerIdleTimerType     = timer.TimerType(1000010)
	playerAutoSaveInterval  = time.Minute
	playerOfflineRetention  = time.Hour
)

type playerAutoSaveTick struct{}

type playerIdleTimeout struct {
	generation uint64
}

// registerTimers 创建玩家定时器，并集中注册各定时器的回调。
func (actor *PlayerActor) registerTimers() {
	actor.wheel = timer.NewTimeWheel(32, actor.selfPID, actor.system)
	actor.wheel.Register(playerAutoSaveTimerType, actor.onAutoSaveTimer)
	actor.wheel.Register(playerIdleTimerType, actor.onIdleTimer)
}

// onAutoSaveTimer 将玩家定时存盘事件投递到角色消息队列。
func (actor *PlayerActor) onAutoSaveTimer(_ interface{}) {
	actor.sendTimerMessage(&playerAutoSaveTick{})
}

// onIdleTimer 校验离线计时器参数，并将回收事件投递到角色消息队列。
func (actor *PlayerActor) onIdleTimer(data interface{}) {
	generation, ok := data.(uint64)
	if !ok {
		tlog.Log.Error("[play/PlayerActor.onIdleTimer] 玩家离线定时器参数无效, pid=%v type=%T", actor.UID, data)
		return
	}
	actor.sendTimerMessage(&playerIdleTimeout{generation: generation})
}

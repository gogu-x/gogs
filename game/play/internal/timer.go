package internal

import (
	"time"

	"github.com/gogu-x/tree/timer"
)

const (
	saveInterval     = 1 * time.Minute
	timerSavePlayers = timer.TimerType(1)
)

// InitTimers 在 Play.Init 中调用，注册模块级定时任务。
func InitTimers(py *Play) {
	py.timeWheel = timer.NewTimeWheel(16, py.TreeCtx().Self(), py.TreeCtx().System())

	py.timeWheel.Register(timerSavePlayers, py.PlayerMgr.OnSaveTimer)
	py.timeWheel.After(timerSavePlayers, saveInterval, "players")

}

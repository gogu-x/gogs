package base

import (
	"time"

	"github.com/gogu-x/tree/timer"
)

const (
	saveInterval     = 1 * time.Minute
	timerSavePlayers = timer.TimerType(1)
)

// InitTimers 在 App.Init 中调用，注册模块级定时任务。
func InitTimers(a *App) {
	a.timeWheel.Register(timerSavePlayers, a.Players.OnSaveTimer)
	a.timeWheel.After(timerSavePlayers, saveInterval, "players")

}

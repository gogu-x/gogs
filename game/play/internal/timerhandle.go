package internal

import (
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/tree/timer"
)

const (
	saveInterval     = 1 * time.Minute
	timerSavePlayers = timer.TimerType(1)
)

// InitTimers 由 core.Play.Boot 调用，注册模块级定时任务。
// 调用时 py.TimeWheel 已由 core.Play.OnInit 创建完成。
func InitTimers(py *core.Play) {
	py.OnTimer(timerSavePlayers, onSavePlayers)
	py.After(timerSavePlayers, saveInterval, nil)
}

// onSavePlayers 定时存盘。
func onSavePlayers(py *core.Play, _ interface{}) {
	py.PlayerMgr.Count()
}

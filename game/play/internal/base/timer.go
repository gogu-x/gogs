package base

import (
	"time"

	"github.com/gogu-x/tree"
)

const saveInterval = 5 * time.Minute

// InitTimers 在 App.Init 中调用，注册模块级定时任务。
func InitTimers(a *App) {
	scheduleSave(a)
}

// scheduleSave 周期性存档，回调在 Play Actor goroutine 内执行，不需要加锁。
func scheduleSave(a *App) {
	if a.ctx == nil {
		return
	}
	a.ctx.AfterFunc(saveInterval, func(_ tree.Context) {
		a.Players.Save()
		scheduleSave(a)
	})
}

package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
)

// InitTimers 由 core.Play.Boot 调用，注册模块级定时任务。
// 调用时 py.TimeWheel 已由 core.Play.OnInit 创建完成。
func InitTimers(py *core.Play) {
}

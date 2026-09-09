package iproto

import (
	"time"

	"github.com/gogu-x/tree"
)

// Params 是 manager 创建 per-battle battle 的构造参数。战报写入所需的
// Repository/Notifier 由装配层注入，battle 自身不感知具体实现。
type Params struct {
	Mode          Mode
	BattleID      string
	UID           uint64
	Source        Source
	Battle        *Battle // Mode==ModeRun 时使用
	Report        Report  // Mode==ModeRedeliver 时使用
	Reports       Repository
	Notifier      Notifier
	RetryDelay    time.Duration
	RetryAttempts int
	ManagerPID    tree.PID // 停前回执的 manager
}

package ctl_chat

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/tree/comm"
)

// OnLogin 处理玩家登录事件；聊天模块尚无登录期逻辑。
func OnLogin(_ *core.PlayerActor, _ *comm.Arg) {}

package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
)

// InitRoutes 注册 play 模块所有路由。
func InitRoutes(py *Play) {
	py.router.Register((*ipb.SessionClosed)(nil), py.onSessionClosed)

	//这里注册可以当前模块消息，也可以注册跨模块的消息，比如注册到actv或者guild，或者cross等等，
	//RegisterPlayer(py, (*pb_chat.ChatReq)(nil), ctl_chat.ChatService)

	//注册回调时的消息路由
	RegisterSys(py, (*pb_auth.LoginGameReq)(nil), ctl_auth.AutoLogin)
}

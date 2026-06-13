package ctl

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoChat"
)

func AutoLogin(a *app.App, _ actor.ActorContext, msg interface{}) {
	//:= msg.(*protoGateway.LoginReq)
	a.Reply(&protoChat.ChatAck{State: 1})
}

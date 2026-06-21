package ctl

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func AutoLogin(a *app.App, _ actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.LoginReq)
	_ = req
	// TODO: 加载玩家数据
	a.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(a *app.App, _ actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.RegisterReq)
	_ = req
	// TODO: 初始化玩家数据
	a.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

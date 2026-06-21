package grpc

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/service"
)

func (a *Actor) onRegister(ctx actor.ActorContext, msg interface{}) {
	resp, err := service.Register(a.mongoPID, msg.(*protoPlatform.RegisterReq))
	ctx.Response(resp, err)
}

func (a *Actor) onLogin(ctx actor.ActorContext, msg interface{}) {
	resp, err := service.Login(a.mongoPID, msg.(*protoPlatform.AuthLoginReq))
	ctx.Response(resp, err)
}

func (a *Actor) onVerify(ctx actor.ActorContext, msg interface{}) {
	resp, err := service.VerifyToken(msg.(*protoPlatform.VerifyTokenReq))
	ctx.Response(resp, err)
}

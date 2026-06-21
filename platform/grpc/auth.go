package grpc

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/service"
)

func (a *Actor) onRegister(ctx actor.ActorContext, msg interface{}) {
	f, mongoPID, req := ctx.Future(), a.mongoPID, msg.(*protoPlatform.RegisterReq)
	go func() {
		resp, err := service.Register(mongoPID, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onLogin(ctx actor.ActorContext, msg interface{}) {
	f, mongoPID, req := ctx.Future(), a.mongoPID, msg.(*protoPlatform.AuthLoginReq)
	go func() {
		resp, err := service.Login(mongoPID, req)
		f.Respond(resp, err)
	}()
}

func (a *Actor) onVerify(ctx actor.ActorContext, msg interface{}) {
	f, req := ctx.Future(), msg.(*protoPlatform.VerifyTokenReq)
	go func() {
		resp, err := service.VerifyToken(req)
		f.Respond(resp, err)
	}()
}

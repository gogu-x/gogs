package grpc

import (
	"github.com/gogu-x/gogs/pb/protoPlatform"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
)

func (a *Platform) onRegister(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.Future(), a.db, msg.(*protoPlatform.RegisterReq)
	go func() {
		resp, err := service.Register(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Platform) onLogin(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.Future(), a.db, msg.(*protoPlatform.AuthLoginReq)
	go func() {
		resp, err := service.Login(db, req)
		f.Respond(resp, err)
	}()
}

func (a *Platform) onVerify(ctx tree.Context, msg interface{}) {
	f, req := ctx.Future(), msg.(*protoPlatform.VerifyTokenReq)
	go func() {
		resp, err := service.VerifyToken(req)
		f.Respond(resp, err)
	}()
}

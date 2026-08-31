package grpc

import (
	"github.com/gogu-x/gogs/pb/pb_pf"
	"github.com/gogu-x/gogs/platform/service"
	"github.com/gogu-x/tree"
)

func (a *Platform) onRegister(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.RequestEnvelope(), a.db, msg.(*pb_pf.RegisterReq)
	resp, err := service.Register(db, req)
	f.Respond(resp, err)
}

func (a *Platform) onLogin(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.RequestEnvelope(), a.db, msg.(*pb_pf.AuthLoginReq)
	resp, err := service.Login(db, req)
	f.Respond(resp, err)
}

func (a *Platform) onVerify(ctx tree.Context, msg interface{}) {
	f, req := ctx.RequestEnvelope(), msg.(*pb_pf.VerifyTokenReq)
	resp, err := service.VerifyToken(req)
	f.Respond(resp, err)
}

func (a *Platform) onGetServerList(ctx tree.Context, msg interface{}) {
	f, db, req := ctx.RequestEnvelope(), a.db, msg.(*pb_pf.GetServerListReq)
	resp, err := service.GetServerList(db, req)
	f.Respond(resp, err)
}

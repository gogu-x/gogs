package app

import (
	actor "github.com/gogu-x/bigTree"
	"google.golang.org/protobuf/proto"
)

// Handle 将 func(*Req) *Resp 注册到 router，省去样板闭包。
// 用法：app.Handle(r, &pb.CreateGuildReq{}, store.Create)
func Handle[Req, Resp proto.Message](r *actor.Router, req Req, fn func(Req) Resp) {
	r.Register(req, func(ctx actor.ActorContext, msg interface{}) {
		ctx.Response(fn(msg.(Req)), nil)
	})
}

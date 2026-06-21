package util

import actor "github.com/gogu-x/bigTree"

// Register 将泛型 handler 注册到 Router，省去类型断言
func Register[Req any](r *actor.Router, req Req, fn func(actor.ActorContext, Req)) {
	r.Register(req, func(ctx actor.ActorContext, msg interface{}) {
		fn(ctx, msg.(Req))
	})
}

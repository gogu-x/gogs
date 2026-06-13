package app

import (
	actor "github.com/gogu-x/bigTree"
)

// Handle 注册 ctl 层处理函数
func (a *App) Handle(fn func(*App, actor.ActorContext, interface{})) actor.Handler {
	return func(ctx actor.ActorContext, msg interface{}) {
		fn(a, ctx, msg)
	}
}

// Register 注册有响应的处理函数，自动调用 ctx.Response
func Register[Req any](r *actor.Router, req Req, fn func(actor.ActorContext, Req)) {
	r.Register(req, func(ctx actor.ActorContext, msg interface{}) {
		fn(ctx, msg.(Req))
	})
}

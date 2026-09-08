package core

import (
	"fmt"

	"github.com/gogu-x/tree"
)

type uidRequest interface {
	GetUID() uint64
}

// RegisterPlayer 注册必须已有在线玩家的 UID 请求。
//
// 校验通过的玩家会直接注入 Context.Player，业务侧无需再查一次。
func RegisterPlayer[Req any](
	py *Play,
	prototype Req,
	h func(*Context, Req),
) {
	py.Router().Register(prototype, func(ctx tree.Context, msg interface{}) {
		request, ok := msg.(uidRequest)
		if !ok || request.GetUID() == 0 {
			// 显式回错：若本条消息是通过 Request 发来的，静默 return 会让
			// 请求方一直等到超时。非请求消息下 Response 是空操作。
			ctx.Response(nil, fmt.Errorf("play: %T missing uid", msg))
			return
		}
		p := py.PlayerMgr.Get(request.GetUID())
		if p == nil {
			ctx.Response(nil, fmt.Errorf("play: uid=%d not online", request.GetUID()))
			return
		}
		h(&Context{
			Play:    py,
			TreeCtx: ctx,
			Req:     msg,
			Player:  p,
		}, msg.(Req))
	})
}

// RegisterSys 注册登录及其他不要求玩家已在线的消息，Context.Player 为 nil。
func RegisterSys[Msg any](
	py *Play,
	prototype Msg,
	h func(*Context, Msg),
) {
	py.Router().Register(prototype, func(ctx tree.Context, msg interface{}) {
		h(&Context{
			Play:    py,
			TreeCtx: ctx,
			Req:     msg,
		}, msg.(Msg))
	})
}

package core

import (
	"fmt"

	"github.com/gogu-x/tree"
)

type uidRequest interface {
	GetUID() uint64
}

// RegisterPlayerMsg 注册必须已有在线玩家的 UID 请求。
//
// 校验通过的玩家会直接注入 Context.Player，业务侧无需再查一次。
func RegisterPlayerMsg[Req any](
	py *Play,
	prototype Req,
	h func(*Context, Req),
) {
	py.Router().Register(prototype, func(ctx tree.Context, msg interface{}) {
		request, ok := msg.(uidRequest)
		if !ok {
			ctx.Response(nil, fmt.Errorf("play: %T missing uid", msg))
			return
		}
		uid := request.GetUID()
		if uid == 0 {
			ctx.Response(nil, fmt.Errorf("play: %T missing uid", msg))
			return
		}
		p := py.PlayerMgr.Get(uid)
		if p == nil || !py.PlayerMgr.IsOnline(uid) {
			ctx.Response(nil, fmt.Errorf("play: uid=%d not online", uid))
			return
		}
		requestCtx := &Context{
			Play:    py,
			TreeCtx: ctx,
			Req:     msg,
			Player:  p,
		}
		h(requestCtx, msg.(Req))
	})
}

// RegisterSysMsg 注册登录及其他不要求玩家已在线的消息，Context.Player 为 nil。
func RegisterSysMsg[Msg any](
	py *Play,
	prototype Msg,
	h func(*Context, Msg),
) {
	py.Router().Register(prototype, func(ctx tree.Context, msg interface{}) {
		requestCtx := &Context{
			Play:    py,
			TreeCtx: ctx,
			Req:     msg,
		}
		h(requestCtx, msg.(Msg))
	})
}

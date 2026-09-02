package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/tree"
)

type uidRequest interface {
	GetUID() uint64
}

// RegisterPlayer 注册必须已有在线玩家的 UID 请求。
func RegisterPlayer[Req any](
	py *Play,
	prototype Req,
	h func(*common.Context, Req),
) {
	py.router.Register(prototype, func(ctx tree.Context, msg interface{}) {
		request, ok := msg.(uidRequest)
		if !ok || request.GetUID() == 0 {
			return
		}
		p := py.PlayerMgr.Get(request.GetUID())
		if p == nil {
			return
		}
		h(&common.Context{
			Play:    py,
			TreeCtx: ctx,
			Player:  p,
		}, msg.(Req))
	})
}

// RegisterSys 注册登录及其他不要求玩家已在线的消息。
func RegisterSys[Msg any](
	py *Play,
	prototype Msg,
	h func(*common.Context, Msg),
) {
	py.router.Register(prototype, func(ctx tree.Context, msg interface{}) {
		h(&common.Context{
			Play:    py,
			TreeCtx: ctx,
		}, msg.(Msg))
	})
}

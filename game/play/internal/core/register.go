package core

import (
	"fmt"

	"github.com/gogu-x/tree"
)

type uidRequest interface {
	// GetUID：返回请求目标玩家的编号。
	GetUID() uint64
}

// RegisterPlayerMsg 注册必须属于当前玩家角色的客户端请求。
func RegisterPlayerMsg[Req any](
	actor *PlayerActor,
	prototype Req,
	handler func(*Context, Req),
) {
	actor.Router().Register(prototype, func(treeCtx tree.Context, msg interface{}) {
		request, ok := msg.(uidRequest)
		if !ok {
			treeCtx.Response(nil, fmt.Errorf("player: %T missing uid", msg))
			return
		}
		uid := request.GetUID()
		if uid == 0 || uid != actor.UID {
			treeCtx.Response(nil, fmt.Errorf("player: request uid=%d does not match actor uid=%d", uid, actor.UID))
			return
		}
		currentPlayer := actor.GetPlayer()
		if currentPlayer == nil {
			treeCtx.Response(nil, fmt.Errorf("player: uid=%d data is not loaded", actor.UID))
			return
		}
		requestCtx := &Context{
			Actor:   actor,
			TreeCtx: treeCtx,
		}
		handler(requestCtx, msg.(Req))
	})
}

// RegisterSysMsg 注册登录及其他系统消息；系统消息仍绑定在当前玩家角色上。
func RegisterSysMsg[Msg any](
	actor *PlayerActor,
	prototype Msg,
	handler func(*Context, Msg),
) {
	actor.Router().Register(prototype, func(treeCtx tree.Context, msg interface{}) {
		requestCtx := &Context{
			Actor:   actor,
			TreeCtx: treeCtx,
		}
		handler(requestCtx, msg.(Msg))
	})
}

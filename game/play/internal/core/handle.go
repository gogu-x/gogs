package core

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
)

// registerMessageHandlers 注册玩家角色的系统消息处理方法。
func (actor *PlayerActor) registerMessageHandlers() {
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.PlayerSessionOpened{},
		actor.onSessionOpened,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.PlayerInbound{},
		actor.onPlayerInbound,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.SessionClosed{},
		actor.onSessionClosed,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&sspb.Event{},
		actor.onServerEvent,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.SavePlayer{},
		actor.onSavePlayer,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.StopAcceptingPlayer{},
		actor.onStopAcceptingPlayer,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.StopPlayerActor{},
		actor.stopIfSaved,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&playerAutoSaveTick{},
		actor.onAutoSaveTick,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&playerIdleTimeout{},
		actor.onIdleTimeout,
	)
	actor.messageRouter.SetFallback(actor.onUnhandledMessage)
}

// HandleMessage 在当前玩家角色内串行处理客户端、存盘和生命周期消息。
func (actor *PlayerActor) HandleMessage(ctx tree.Context, message interface{}) {
	actor.messageRouter.Route(ctx, message)
}

// onUnhandledMessage 将未注册的系统消息交给加载队列或业务路由处理。
func (actor *PlayerActor) onUnhandledMessage(ctx tree.Context, message interface{}) {
	if !actor.ready {
		actor.queueWhileLoading(ctx, message)
		return
	}
	actor.routeMessage(ctx, message)
}

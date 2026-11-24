package core

import (
	"fmt"

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
	if delivery, ok := message.(*ipb.DeliveryWithReply); ok {
		actor.routeDeliveryWithReply(ctx, delivery)
		return
	}
	actor.routeMessage(ctx, message)
}

// routeDeliveryWithReply 执行需要回复原调用方的服务端业务投递。
//
// 业务处理函数通过 ctx.Response 把结果回给原调用方。处理函数 panic 时先向调用方回复错误
// 再继续向上抛出，由框架记录堆栈；否则调用方会一直等待一个永远不会到来的结果。
func (actor *PlayerActor) routeDeliveryWithReply(ctx tree.Context, delivery *ipb.DeliveryWithReply) {
	defer func() {
		if recovered := recover(); recovered != nil {
			delivery.Reply(nil, fmt.Errorf("player %d handler panic: %v", actor.UID, recovered))
			panic(recovered)
		}
	}()
	actor.routeMessage(delivery.ReplyContext(ctx), delivery.GetPayload())
}

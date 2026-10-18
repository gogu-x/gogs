package playeractor

import (
	"fmt"

	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// registerMessageHandlers 注册玩家活动 Actor 的系统消息处理方法。
func (actor *Actor) registerMessageHandlers() {
	tree.RegisterFunc(
		&actor.messageRouter,
		&sspb.Event{},
		actor.onServerEvent,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.PlayerSessionOpened{},
		actor.onSessionOpened,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.SessionClosed{},
		actor.onSessionClosed,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.PlayerInbound{},
		actor.onPlayerInbound,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.PlayerActivityMessage{},
		actor.onPlayerActivityMessage,
	)
	tree.RegisterFunc(
		&actor.messageRouter,
		&ipb.SavePlayerActivities{},
		actor.onSavePlayerActivities,
	)
	actor.messageRouter.SetFallback(actor.routeOrQueue)
}

// registerRoutes 注册玩家活动协议请求的业务处理方法。
func (actor *Actor) registerRoutes() {
	tree.RegisterFunc(
		&actor.router,
		&pb_activity.GetActivityListReq{},
		actor.getActivityList,
	)
	tree.RegisterFunc(
		&actor.router,
		&pb_activity.JoinActivityReq{},
		actor.joinActivity,
	)
	tree.RegisterFunc(
		&actor.router,
		&pb_activity.GetProgressReq{},
		actor.getProgress,
	)
	tree.RegisterFunc(
		&actor.router,
		&pb_activity.ClaimRewardReq{},
		actor.claimReward,
	)
}

// HandleMessage routes player activity messages to their registered handlers.
func (actor *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	actor.messageRouter.Route(ctx, message)
}

// onServerEvent 处理玩家活动 Actor 收到的服务器生命周期事件。
func (actor *Actor) onServerEvent(ctx tree.Context, message *sspb.Event) {
	if gameevent.IsServerOpened(message) {
		actor.serverOpened = true
		tlog.Log.Info("[activity/playeractor.onServerEvent] 玩家活动 Actor 已处理开服事件, pid=%v", actor.UID)
		ctx.Response(nil, nil)
		return
	}
	if !gameevent.IsServerClosing(message) {
		eventName := message.GetName()
		err := fmt.Errorf("unsupported player activity event %q", eventName)
		tlog.Log.Error("[activity/playeractor.onServerEvent] 收到不支持的玩家活动事件, event=%v pid=%v err=%v", eventName, actor.UID, err)
		ctx.Response(nil, err)
		return
	}
	actor.serverClosing = true
	tlog.Log.Info("[activity/playeractor.onServerEvent] 玩家活动 Actor 已接收关服事件, pid=%v", actor.UID)
	if !actor.ready {
		request := ctx.RequestEnvelope()
		if actor.serverCloseRequest != nil {
			if request != nil {
				request.Respond(nil, fmt.Errorf("player activity shutdown is already pending"))
			}
			return
		}
		actor.serverCloseRequest = request
		return
	}
	actor.saveAll(ctx, ctx.RequestEnvelope())
}

// onPlayerActivityMessage 将目标属于当前玩家的活动命令交给业务路由。
func (actor *Actor) onPlayerActivityMessage(ctx tree.Context, message *ipb.PlayerActivityMessage) {
	if message == nil {
		return
	}
	if uid := message.GetUID(); uid == actor.UID {
		actor.routeOrQueue(ctx, message.GetMessage())
	}
}

// onSavePlayerActivities 处理当前玩家活动数据的存盘请求。
func (actor *Actor) onSavePlayerActivities(ctx tree.Context, message *ipb.SavePlayerActivities) {
	if message == nil {
		return
	}
	if uid := message.GetUID(); uid == 0 || uid == actor.UID {
		actor.saveAll(ctx, ctx.RequestEnvelope())
	}
}

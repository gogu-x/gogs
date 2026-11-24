package internal

import (
	"fmt"

	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// registerHandlers 注册联盟 Actor 的消息处理器；客户端请求统一经 ForwardedRequest 到达。
func (guild *GuildActor) registerHandlers() {
	router := &guild.messageRouter
	tree.RegisterFunc(router, &ipb.ForwardedRequest{}, guild.onForwardedRequest)
	tree.RegisterFunc(router, &ipb.SyncPlayerGuild{}, guild.onSyncPlayerGuild)
	tree.RegisterFunc(router, &sspb.Event{}, guild.onServerEvent)
	tree.RegisterFunc(router, &ipb.SaveAllGuilds{}, guild.onSaveAllGuilds)
}

// onForwardedRequest 处理玩家 Actor 转交的联盟请求：联盟数据加载期间按收件顺序暂存，
// 就绪后执行业务并按请求的会话版本回包；联盟数据不可用时推送 ErrorNtf(INTERNAL)。
func (guild *GuildActor) onForwardedRequest(ctx tree.Context, forwarded *ipb.ForwardedRequest) {
	uid, generation := forwarded.GetUID(), forwarded.GetGeneration()
	if uid == 0 || generation == 0 || forwarded.GetRequest() == nil {
		tlog.Log.Error("[guild/GuildActor.onForwardedRequest] 联盟请求或会话无效, pid=%v generation=%v", uid, generation)
		return
	}
	guild.dispatchRequest(ctx, uid, func(ctx tree.Context, available bool) {
		request := forwarded.GetRequest()
		var response proto.Message
		if available {
			response = guild.handleClientRequest(ctx, uid, request)
		}
		if response == nil {
			response = ipb.NewErrorNotification(request, pb_common.ErrCode_INTERNAL)
		}
		ipb.ReplyToSession(ctx, uid, generation, response)
	})
}

// handleClientRequest 执行一条联盟客户端请求并返回响应；不支持的请求类型返回 nil。
// 创建、加入、退出成功后同步玩家联盟归属，创建成功时还会通知活动服务加载联盟活动。
func (guild *GuildActor) handleClientRequest(ctx tree.Context, uid uint64, request proto.Message) proto.Message {
	switch request := request.(type) {
	case *pb_guild.CreateGuildReq:
		ack := guild.store.Create(request)
		if owner := ack.GetGuild(); ack.GetCode() == pb_common.ErrCode_OK && owner != nil {
			guild.sendPlayerGuildMembership(ctx, uid)
			guild.notifyActvOwnerCreated(ctx, owner.GetGuildId())
		}
		return ack
	case *pb_guild.JoinGuildReq:
		ack := guild.store.Join(request)
		if ack.GetCode() == pb_common.ErrCode_OK {
			guild.sendPlayerGuildMembership(ctx, uid)
		}
		return ack
	case *pb_guild.LeaveGuildReq:
		ack := guild.store.Leave(request)
		if ack.GetCode() == pb_common.ErrCode_OK {
			guild.sendPlayerGuildMembership(ctx, uid)
		}
		return ack
	case *pb_guild.GetGuildReq:
		return guild.store.Get(request)
	}
	tlog.Log.Error("[guild/GuildActor.handleClientRequest] 不支持的联盟请求, pid=%v type=%T", uid, request)
	return nil
}

// dispatchRequest 联盟数据就绪时立即执行，加载期间按收件顺序暂存；available 表示联盟数据是否可用。
func (guild *GuildActor) dispatchRequest(ctx tree.Context, uid uint64, handle func(tree.Context, bool)) {
	if guild.serverClosing {
		handle(ctx, false)
		return
	}
	dispatch := func(ctx tree.Context) {
		handle(ctx, guild.loadErr == nil)
	}
	if guild.ready {
		dispatch(ctx)
		return
	}
	if !guild.addPending(dispatch) {
		tlog.Log.Error("[guild/GuildActor.dispatchRequest] 联盟加载等待队列已满, pid=%v", uid)
		handle(ctx, false)
	}
}

// onSyncPlayerGuild 用当前成员索引修正登录玩家的联盟归属。
func (guild *GuildActor) onSyncPlayerGuild(ctx tree.Context, request *ipb.SyncPlayerGuild) {
	uid := request.GetUID()
	if uid == 0 {
		tlog.Log.Error("[guild/GuildActor.onSyncPlayerGuild] 联盟归属同步请求缺少玩家编号")
		return
	}
	guild.dispatchRequest(ctx, uid, func(ctx tree.Context, available bool) {
		if !available {
			tlog.Log.Error("[guild/GuildActor.onSyncPlayerGuild] 联盟归属暂不可同步, pid=%v", uid)
			return
		}
		guild.sendPlayerGuildMembership(ctx, uid)
	})
}

// onServerEvent 处理服务器开服和关服事件。
func (guild *GuildActor) onServerEvent(ctx tree.Context, message *sspb.Event) {
	if gameevent.IsServerOpened(message) {
		guild.serverOpened = true
		tlog.Log.Info("[guild/GuildActor.onServerEvent] 联盟 Actor 已处理开服事件")
		ctx.Response(nil, nil)
		return
	}
	if !gameevent.IsServerClosing(message) {
		eventName := message.GetName()
		err := fmt.Errorf("unsupported guild event %q", eventName)
		tlog.Log.Error("[guild/GuildActor.onServerEvent] 收到不支持的联盟事件, event=%v err=%v", eventName, err)
		ctx.Response(nil, err)
		return
	}
	guild.serverClosing = true
	tlog.Log.Info("[guild/GuildActor.onServerEvent] 联盟 Actor 已接收关服事件")
	request := ctx.RequestEnvelope()
	if guild.ready || guild.loadErr != nil {
		guild.saveAllRequest(ctx, request)
		return
	}
	if request == nil {
		tlog.Log.Error("[guild/GuildActor.onServerEvent] 联盟加载期间收到无请求信封的关服事件")
		return
	}
	if !guild.addPending(func(ctx tree.Context) {
		if err := guild.loadErr; err != nil {
			request.Respond(nil, err)
			return
		}
		guild.saveAllRequest(ctx, request)
	}) {
		request.Respond(nil, fmt.Errorf("guild shutdown queue is full"))
	}
}

// onSaveAllGuilds 处理保存全部联盟的系统请求。
func (guild *GuildActor) onSaveAllGuilds(ctx tree.Context, _ *ipb.SaveAllGuilds) {
	guild.saveAllRequest(ctx, ctx.RequestEnvelope())
}

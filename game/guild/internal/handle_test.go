package internal

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/guild/internal/store"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// guildPushContext 记录联盟 Actor 对会话代理和活动 Actor 的投递。
type guildPushContext struct {
	tree.Context
	agent   tree.PID
	actv    tree.PID
	pushed  *ipb.PushToMsg
	target  tree.PID
	created *ipb.CreateGuildActvOwner
	members []*ipb.PlayerGuildMembership
}

func (ctx *guildPushContext) Lookup(name string) (tree.PID, bool) {
	if name == ctx.agent.Name {
		return ctx.agent, true
	}
	if name == def.GameGate {
		return tree.PID{ID: 3, Name: def.GameGate}, true
	}
	return ctx.actv, name == def.ActorActv && ctx.actv.ID != 0
}

func (ctx *guildPushContext) TrySend(pid tree.PID, message interface{}) bool {
	if pid.Name == def.GameGate {
		delivery := message.(*ipb.DeliverToPlayer)
		ctx.members = append(ctx.members, delivery.Payload.(*ipb.PlayerGuildMembership))
		return true
	}
	ctx.target = pid
	ctx.pushed = message.(*ipb.PushToMsg)
	return true
}

func (ctx *guildPushContext) Send(pid tree.PID, message interface{}) bool {
	if pid != ctx.actv {
		return false
	}
	ctx.created = message.(*ipb.CreateGuildActvOwner)
	return true
}

// forwardGuild 模拟玩家 Actor 以指定会话代次转交联盟请求。
func forwardGuild(uid, generation uint64, request proto.Message) *ipb.ForwardedRequest {
	return &ipb.ForwardedRequest{UID: uid, Generation: generation, Request: request}
}

// guildErrorNtf 返回联盟不可用时请求对应的通用错误通知。
func guildErrorNtf(request proto.Message) *pb_common.ErrorNtf {
	return ipb.NewErrorNotification(request, pb_common.ErrCode_INTERNAL)
}

func assertGuildPush(
	t *testing.T,
	ctx *guildPushContext,
	uid uint64,
	generation uint64,
	wantAck interface{},
	wantCode pb_common.ErrCode,
) {
	t.Helper()
	push := ctx.pushed
	if push == nil || ctx.target != ctx.agent || push.GetUID() != uid || push.GetGeneration() != generation {
		t.Fatalf("会话推送不匹配，目标=%v，消息=%+v", ctx.target, push)
	}
	if ntf, isError := push.Msg.(*pb_common.ErrorNtf); isError {
		if wantNtf, ok := wantAck.(*pb_common.ErrorNtf); !ok || ntf.GetCode() != wantCode || ntf.GetRequestType() != wantNtf.GetRequestType() {
			t.Fatalf("收到错误通知 %+v，期望 %T 和错误码 %v", ntf, wantAck, wantCode)
		}
		ctx.pushed = nil
		return
	}
	ack, ok := push.Msg.(interface{ GetCode() pb_common.ErrCode })
	if !ok || reflect.TypeOf(ack) != reflect.TypeOf(wantAck) || ack.GetCode() != wantCode {
		t.Fatalf("收到响应 %T %+v，期望 %T 和错误码 %v", ack, ack, wantAck, wantCode)
	}
	ctx.pushed = nil
}

// TestGuildClientRequestsPushDirectly 验证四个注册入口均向原请求会话直接回包。
func TestGuildClientRequestsPushDirectly(t *testing.T) {
	guild := NewGuildActor()
	guild.ready = true
	guild.registerHandlers()
	ctx := &guildPushContext{actv: tree.PID{ID: 1, Name: def.ActorActv}}
	tests := []struct {
		uid     uint64
		message proto.Message
		ack     interface{}
	}{
		{7, &pb_guild.CreateGuildReq{UID: 7, Name: "联盟"}, &pb_guild.CreateGuildAck{}},
		{8, &pb_guild.JoinGuildReq{UID: 8, GuildId: 1}, &pb_guild.JoinGuildAck{}},
		{8, &pb_guild.GetGuildReq{UID: 8, GuildId: 1}, &pb_guild.GetGuildAck{}},
		{8, &pb_guild.LeaveGuildReq{UID: 8}, &pb_guild.LeaveGuildAck{}},
	}
	for _, tt := range tests {
		ctx.agent = tree.PID{ID: 2, Name: def.SessionAgentName(tt.uid)}
		guild.HandleMessage(ctx, forwardGuild(tt.uid, 3, tt.message))
		assertGuildPush(
			t,
			ctx,
			tt.uid,
			3,
			tt.ack,
			pb_common.ErrCode_OK,
		)
	}
	if ctx.created == nil || ctx.created.GuildID != 1 {
		t.Fatal("创建联盟后应通知活动 Actor")
	}
	guild.HandleMessage(ctx, &ipb.SyncPlayerGuild{UID: 7})
	wantMembership := []*ipb.PlayerGuildMembership{
		{UID: 7, GuildID: 1},
		{UID: 8, GuildID: 1},
		{UID: 8, GuildID: 0},
		{UID: 7, GuildID: 1},
	}
	if !reflect.DeepEqual(ctx.members, wantMembership) {
		t.Fatalf("联盟归属更新不匹配，收到 %+v", ctx.members)
	}
}

// TestGuildPendingClientRequest 验证加载结束后仍按原请求代次回包。
func TestGuildPendingClientRequest(t *testing.T) {
	for _, loadErr := range []error{nil, errors.New("联盟加载失败")} {
		guild := NewGuildActor()
		guild.registerHandlers()
		ctx := &guildPushContext{agent: tree.PID{ID: 1, Name: def.SessionAgentName(7)}}
		message := &pb_guild.GetGuildReq{UID: 7, GuildId: 1}
		guild.HandleMessage(ctx, forwardGuild(7, 3, message))
		if ctx.pushed != nil || len(guild.pending) != 1 {
			t.Fatal("加载期间应先暂存普通请求")
		}

		// 当前代理已经换线，推送仍携带发起请求时的会话代次。
		ctx.agent.ID = 2
		var wantAck interface{} = &pb_guild.GetGuildAck{}
		wantCode := pb_common.ErrCode_GUILD_NOT_FOUND
		if loadErr != nil {
			wantAck, wantCode = guildErrorNtf(message), pb_common.ErrCode_INTERNAL
		}
		guild.onGuildsLoaded(ctx, &[]store.Guild{}, loadErr)
		assertGuildPush(t, ctx, 7, 3, wantAck, wantCode)
	}
}

// TestGuildUnavailablePush 验证加载队列已满或关服时推送通用错误通知。
func TestGuildUnavailablePush(t *testing.T) {
	message := &pb_guild.GetGuildReq{UID: 7, GuildId: 1}
	guild := NewGuildActor()
	guild.registerHandlers()
	ctx := &guildPushContext{agent: tree.PID{ID: 1, Name: def.SessionAgentName(7)}}
	guild.pending = make([]func(tree.Context), maxPendingGuildRequests)
	guild.HandleMessage(ctx, forwardGuild(7, 3, message))
	assertGuildPush(t, ctx, 7, 3, guildErrorNtf(message), pb_common.ErrCode_INTERNAL)

	guild.pending = nil
	guild.ready = true
	guild.serverClosing = true
	guild.HandleMessage(ctx, forwardGuild(7, 3, message))
	assertGuildPush(t, ctx, 7, 3, guildErrorNtf(message), pb_common.ErrCode_INTERNAL)
}

package core

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// ForwardPlayerMsg 注册玩家侧没有业务、直接转交给 target Actor 处理的客户端请求。
//
// 复用 RegisterPlayerMsg 的 UID 与数据就绪校验，再把请求包装成 ipb.ForwardedRequest 非阻塞投递；
// 目标 Actor 不可用、队列已满或当前没有在线会话时，向客户端推送 ErrorNtf(INTERNAL)。
// 业务 Actor 负责按请求中的会话版本回包。
func ForwardPlayerMsg[Req proto.Message](actor *PlayerActor, prototype Req, target string) {
	RegisterPlayerMsg(actor, prototype, func(ctx *Context, request Req) {
		forwardToActor(ctx, target, request)
	})
}

// forwardToActor 把当前玩家的客户端请求转交给 target Actor。
func forwardToActor(ctx *Context, target string, request proto.Message) {
	uid := ctx.PlayerID()
	generation := ctx.SessionGeneration()
	if !ctx.SessionActive() || generation == 0 {
		tlog.Log.Error("[play/forwardToActor] 没有在线会话，转发请求已丢弃, pid=%v target=%v type=%T", uid, target, request)
		return
	}
	forwarded := &ipb.ForwardedRequest{UID: uid, Generation: generation, Request: request}
	if pid, ok := ctx.TreeCtx.Lookup(target); ok && ctx.TreeCtx.TrySend(pid, forwarded) {
		return
	}
	tlog.Log.Error("[play/forwardToActor] 目标 Actor 不可用或队列已满, pid=%v target=%v type=%T", uid, target, request)
	ipb.ReplyToSession(ctx.TreeCtx, uid, generation, ipb.NewErrorNotification(request, pb_common.ErrCode_INTERNAL))
}

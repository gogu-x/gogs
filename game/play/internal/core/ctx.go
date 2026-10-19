package core

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"google.golang.org/protobuf/proto"
)

// Context 是单玩家角色同步处理一条消息时使用的请求上下文。
//
// 底层上下文只在本条消息的处理过程中有效，不得保存到异步回调或玩家角色状态中。
type Context struct {
	Actor   *PlayerActor // 当前处理消息的玩家 Actor。
	TreeCtx tree.Context // 当前消息对应的 Tree Context。
}

// GetPlayer 返回当前请求绑定的玩家数据。
func (ctx *Context) GetPlayer() *player.Player {
	if ctx == nil || ctx.Actor == nil {
		return nil
	}
	return ctx.Actor.GetPlayer()
}

// PlayerID 返回当前请求的玩家编号；无玩家数据时返回 0。
func (ctx *Context) PlayerID() uint64 {
	currentPlayer := ctx.GetPlayer()
	if currentPlayer == nil {
		return 0
	}
	return currentPlayer.GetUID()
}

// SessionActive 返回当前玩家请求是否来自有效网关会话。
func (ctx *Context) SessionActive() bool {
	return ctx != nil && ctx.Actor != nil && ctx.Actor.sessionActive
}

// SessionGeneration 返回当前玩家 Actor 绑定的网关会话版本。
func (ctx *Context) SessionGeneration() uint64 {
	if ctx == nil || ctx.Actor == nil {
		return 0
	}
	return ctx.Actor.sessionGeneration
}

// Response 回复当前请求；普通网关消息没有请求信封时不执行任何操作。
func (ctx *Context) Response(value interface{}, err error) {
	if ctx != nil && ctx.TreeCtx != nil {
		ctx.TreeCtx.Response(value, err)
	}
}

// Cast 按角色名称投递普通消息。
func (ctx *Context) Cast(name string, msg interface{}) bool {
	if ctx == nil || ctx.TreeCtx == nil {
		return false
	}
	pid, ok := ctx.TreeCtx.Lookup(name)
	if !ok {
		return false
	}
	return ctx.TreeCtx.Send(pid, msg)
}

// CastPID 向指定进程编号投递普通消息。
func (ctx *Context) CastPID(pid tree.PID, msg interface{}) bool {
	return ctx != nil && ctx.TreeCtx != nil && ctx.TreeCtx.Send(pid, msg)
}

// CastCall 投递带异步回调的请求，回调在当前玩家角色所属协程中执行。
func (ctx *Context) CastCall(
	name string,
	msg interface{},
	callback func(tree.Context, interface{}, error),
) bool {
	if ctx == nil || ctx.TreeCtx == nil {
		return false
	}
	pid, ok := ctx.TreeCtx.Lookup(name)
	if !ok {
		return false
	}
	return ctx.TreeCtx.RequestCallback(pid, msg, callback) != nil
}

// CastPlayerMsg 通过网关向在线玩家推送协议消息。
func (ctx *Context) CastPlayerMsg(uid uint64, msg proto.Message) bool {
	if ctx == nil || ctx.TreeCtx == nil || uid == 0 || msg == nil {
		return false
	}
	gatePID, ok := ctx.TreeCtx.Lookup(def.GameGate)
	if !ok {
		return false
	}
	return ctx.TreeCtx.Send(gatePID, &ipb.PushToMsg{UID: uid, Msg: msg})
}

// Emit 同步触发当前玩家角色的业务事件。
func (ctx *Context) Emit(name comm.EventName, arg *comm.Arg) int {
	if ctx == nil || ctx.Actor == nil {
		return 0
	}
	return ctx.Actor.Emit(name, arg)
}

// MarkDirty 标记当前玩家数据需要存盘。
func (ctx *Context) MarkDirty() {
	if currentPlayer := ctx.GetPlayer(); currentPlayer != nil {
		currentPlayer.MarkDirty()
	}
}

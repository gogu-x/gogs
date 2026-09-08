package core

import (
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
	"google.golang.org/protobuf/proto"
)

// Context 是一次请求的上下文。
//
// 生命周期严格限于当前这条消息：TreeCtx 携带本条消息的 sender / request / values，
// 一旦 *Context 逃逸到定时器回调、事件参数或异步 goroutine 中，Response 会失效，
// 或错误地回复到一个已经完成的请求上。因此禁止保存 *Context，
// 需要跨消息传递时只传 uid 等值类型。
type Context struct {
	// Play 是本节点的运行期状态与公共能力。
	Play *Play
	// TreeCtx 是本条消息独有的 Actor context。
	TreeCtx tree.Context
	// Req 是原始请求消息。
	Req interface{}
	// Player 是本次请求的玩家，由 RegisterPlayer 预先校验并注入；
	// RegisterSys 注册的消息（如登录）下为 nil。
	Player *player.Player
}

// 公共能力访问器：直接读字段，无额外分配。

func (c *Context) Players() *player.PlayerMgr          { return c.Play.PlayerMgr }
func (c *Context) Event() *comm.Event                  { return c.Play.Event }
func (c *Context) TimeWheel() *timer.TimeWheel         { return c.Play.TimeWheel }
func (c *Context) Tree() tree.Context                  { return c.TreeCtx }
func (c *Context) GetPlayer(uid uint64) *player.Player { return c.Play.PlayerMgr.Get(uid) }

// PlayerID 返回本次请求的玩家 ID；无玩家上下文时返回 0。
func (c *Context) PlayerID() uint64 {
	if c.Player == nil {
		return 0
	}
	return c.Player.UID
}

// Response 回复本条请求。若本条消息不是通过 Request 发来的，则为空操作。
func (c *Context) Response(value interface{}, err error) {
	c.TreeCtx.Response(value, err)
}

// Cast 按名字查找 Actor 并投递普通消息，目标不存在返回 false。
func (c *Context) Cast(name string, msg interface{}) bool {
	pid, ok := c.TreeCtx.Lookup(name)
	if !ok {
		return false
	}
	return c.TreeCtx.Send(pid, msg)
}

// CastPID 向指定 PID 投递普通消息。
func (c *Context) CastPID(pid tree.PID, msg interface{}) bool {
	return c.TreeCtx.Send(pid, msg)
}

// CastCall 投递一个带回调的请求，回调在本 Actor 的 goroutine 内执行。
func (c *Context) CastCall(
	name string,
	msg interface{},
	cb func(tree.Context, interface{}, error),
) bool {
	pid, ok := c.TreeCtx.Lookup(name)
	if !ok {
		return false
	}
	c.TreeCtx.RequestCallback(pid, msg, cb)
	return true
}

// CastPlayerIdMsg 通过网关向指定在线玩家推送消息，玩家不在线返回 false。
func (c *Context) CastPlayerIdMsg(uid uint64, msg proto.Message) bool {
	p := c.Play.PlayerMgr.Get(uid)
	if p == nil {
		return false
	}
	return c.Cast(def.GameGate, &ipb.PushToMsg{UID: p.UID, Msg: msg})
}

// Emit 同步触发事件，监听者拿到的就是当前这个 Context——因此监听者内部
// 仍然可以 Response、仍然能透传 trace values。分发在本 goroutine 内完成，
// 返回时上下文绑定已解除，监听者不得留存 ctx。
func (c *Context) Emit(name comm.EventName, arg *comm.Arg) int {
	return c.Play.emitWithContext(c, name, arg)
}

// After 挂一条一次性延时任务。注意定时回调拿到的是 SysCtx 而不是本 Context，
// 需要跨到回调里的数据请通过 data 传值（如 uid），不要闭包捕获 *Context。
func (c *Context) After(timerType timer.TimerType, d time.Duration, data interface{}) *timer.WheelTimer {
	return c.Play.TimeWheel.After(timerType, d, data)
}

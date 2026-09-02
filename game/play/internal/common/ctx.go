// Package common 提供 play 模块共享的玩家上下文与公共能力集合。
package common

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
	"google.golang.org/protobuf/proto"
)

// PlayerHandler 处理来自 gate 的玩家请求，上下文携带玩家数据。
type PlayerHandler func(*Context, interface{})

// SysHandler 处理系统或登录消息。
type SysHandler func(*Context, interface{})

// Services 是 Play 持有的公共能力集合。
type Services struct {
	Players   *player.PlayerMgr
	Event     *comm.Event
	TimeWheel *timer.TimeWheel
}

// PlayIface 是 Play 向业务层暴露能力的稳定接口。
type PlayIface interface {
	Services() *Services
}

// Context 是一次请求的上下文。
type Context struct {
	Play    PlayIface
	TreeCtx tree.Context
	Player  *player.Player
}

func (c *Context) Services() *Services         { return c.Play.Services() }
func (c *Context) Players() *player.PlayerMgr  { return c.Services().Players }
func (c *Context) Event() *comm.Event          { return c.Services().Event }
func (c *Context) TimeWheel() *timer.TimeWheel { return c.Services().TimeWheel }
func (c *Context) Tree() tree.Context          { return c.TreeCtx }

// CastCall 投递一个带有回调的消息
func (c *Context) CastCall(
	a string,
	msg interface{},
	cb func(tree.Context, interface{}, error),
) bool {
	pid, ok := c.Tree().Lookup(a)
	if !ok {
		return false
	}
	c.TreeCtx.RequestCallback(pid, msg, cb)
	return true
}

// CastPID 投递一个普通消息
func (c *Context) CastPID(pid tree.PID, msg interface{}) bool {
	return c.TreeCtx.Send(pid, msg)
}

// Cast 投递一个普通消息
func (c *Context) Cast(a string, msg interface{}) bool {
	pid, ok := c.Tree().Lookup(a)
	if !ok {
		return false
	}
	return c.TreeCtx.Send(pid, msg)
}

func (c *Context) Response(value interface{}, err error) {
	c.TreeCtx.Response(value, err)
}

// CastPlayerIdMsg 给玩家投递消息 网关
func (c *Context) CastPlayerIdMsg(uid uint64, msg proto.Message) {
	p := c.Players().Get(uid)
	if p == nil {
		return
	}
	c.Cast(def.Gate, &ipb.PushToMsg{UID: p.UID, Msg: msg})
}

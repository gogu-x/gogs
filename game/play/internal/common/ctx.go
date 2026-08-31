// Package common 提供 play 模块共享的玩家上下文与公共能力集合。
package common

import (
	"log"

	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
	"google.golang.org/protobuf/proto"
)

// PlayerHandler 处理来自 gate 的玩家请求，上下文携带玩家数据。
type PlayerHandler func(*PlayerContext, interface{})

// SysHandler 处理系统或登录消息。
type SysHandler func(*PlayerContext, interface{})

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

// PlayerContext 是一次请求的上下文。
type PlayerContext struct {
	Play    PlayIface
	TreeCtx tree.Context
	Player  *player.Player
}

func (c *PlayerContext) Services() *Services         { return c.Play.Services() }
func (c *PlayerContext) Players() *player.PlayerMgr  { return c.Services().Players }
func (c *PlayerContext) Event() *comm.Event          { return c.Services().Event }
func (c *PlayerContext) TimeWheel() *timer.TimeWheel { return c.Services().TimeWheel }
func (c *PlayerContext) Tree() tree.Context          { return c.TreeCtx }

func (c *PlayerContext) Request(
	pid tree.PID,
	msg interface{},
	cb func(tree.Context, interface{}, error),
) {
	c.TreeCtx.RequestCallback(pid, msg, cb)
}

func (c *PlayerContext) Send(pid tree.PID, msg interface{}) bool {
	return c.TreeCtx.Send(pid, msg)
}

func (c *PlayerContext) Response(value interface{}, err error) {
	c.TreeCtx.Response(value, err)
}

// Reply serializes a response and sends it back to the GsAgent that sent the request.
func (c *PlayerContext) Reply(msg proto.Message) {
	payload, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("play: marshal reply %T: %v", msg, err)
		return
	}
	if !c.TreeCtx.Send(c.TreeCtx.Sender(), &pb_gateway.Frame{Payload: payload}) {
		log.Printf("play: reply sender unavailable, message=%T", msg)
	}
}

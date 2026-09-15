package conn

import (
	"reflect"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/tlog"
	"github.com/gorilla/websocket"
)

func initHandler(c *Conn) {
	c.router.Register(&WsMsg{}, c.onWsMsg)
	c.router.Register(&stopMsg{}, c.onStop)
	c.router.Register(&streamClosed{}, c.onStreamClosed)

	c.router.Register(&pb_gateway.Frame{}, c.onFrame)
	c.router.Register(&pb_gateway.BroadcastMsg{}, c.onBroadcast)
	c.router.Register(&pb_gateway.LoginReq{}, c.onLogin)
	c.router.Register(&pb_gateway.RegisterReq{}, c.onRegister)
	c.router.Register(&pb_gateway.GetServerListReq{}, c.onGetServerList)
}

func (c *Conn) onWsMsg(ctx tree.Context, msg interface{}) {
	inner, err := c.codec.Unmarshal(msg.(*WsMsg).Data)
	if err != nil {
		tlog.Log.Info("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}
	tlog.Log.Info("gate conn onWsMsg: %v", inner)
	msgType := reflect.TypeOf(inner)
	if hookHandler, ok := c.hook[msgType]; ok {
		hookHandler(ctx, inner)
		return
	}

	if c.state == StateLoggIng {
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_LOGIN_IN_PROGRESS, Msg: "login in progress"})
		return
	}
	if c.state != StateAuthed {
		c.WriteWsMsg(&pb_gateway.LoginAck{Code: pb_common.ErrCode_UNAUTHORIZED, Msg: "unauthorized"})
		return
	}
	c.router.SetFallback(func(ctx tree.Context, _ interface{}) {
		c.sendGameSteam(ctx, inner)
	})
	c.router.Route(ctx, inner)
}

func (c *Conn) onFrame(ctx tree.Context, msg interface{}) {
	frame := msg.(*pb_gateway.Frame)
	if len(frame.GetPayload()) == 0 {
		return
	}
	inner, err := codec.ProtoCodec.Unmarshal(frame.GetPayload())
	if err != nil {
		tlog.Log.Info("ConnActor[%d]: decode game payload: %v", c.connID, err)
		return
	}
	msgType := reflect.TypeOf(inner)
	if hookHandler, ok := c.hook[msgType]; ok {
		hookHandler(ctx, inner)
		return
	}

	//写入ws消息流
	if err := c.conn.WriteMessage(websocket.BinaryMessage, frame.GetPayload()); err != nil {
		tlog.Log.Info("ConnActor[%d]: ws write error: %v", c.connID, err)
		ctx.Stop()
	}
}

func (c *Conn) onBroadcast(_ tree.Context, msg interface{}) {
	_ = c.conn.WriteMessage(websocket.BinaryMessage, msg.(*pb_gateway.BroadcastMsg).Data)
}

func (c *Conn) onStop(ctx tree.Context, _ interface{}) {
	ctx.Stop()
}

func (c *Conn) onStreamClosed(ctx tree.Context, msg interface{}) {
	if err := msg.(*streamClosed).err; err != nil {
		tlog.Log.Info("ConnActor[%d]: game stream closed: %v", c.connID, err)
	}
	ctx.Stop()
}

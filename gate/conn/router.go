package conn

import (
	"log"

	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gorilla/websocket"
)

func initRouter(c *Conn) {
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
		log.Printf("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}
	for _, mw := range c.middlewares {
		if !mw(ctx, inner) {
			return
		}
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
		log.Printf("ConnActor[%d]: decode game payload: %v", c.connID, err)
		return
	}
	if ack, ok := inner.(*pb_gateway.LoginAck); ok {
		if ack.GetCode() == pb_common.ErrCode_OK {
			c.state = StateAuthed
		} else {
			c.state = StateAnon
		}
	}
	data, err := c.codec.Marshal(inner)
	if err != nil {
		log.Printf("ConnActor[%d]: encode websocket reply: %v", c.connID, err)
		return
	}
	if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		log.Printf("ConnActor[%d]: ws write error: %v", c.connID, err)
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
		log.Printf("ConnActor[%d]: game stream closed: %v", c.connID, err)
	}
	ctx.Stop()
}

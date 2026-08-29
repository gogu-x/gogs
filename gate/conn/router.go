package conn

import (
	"log"

	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"github.com/gorilla/websocket"
)

func initRouter(c *Conn) {
	c.router.Register(&WsMsg{}, c.onWsMsg)
	c.router.Register(&stopMsg{}, c.onStop)
	c.router.Register(&streamClosed{}, c.onStreamClosed)

	c.router.Register(&protoGateway.Frame{}, c.onFrame)
	c.router.Register(&protoGateway.BroadcastMsg{}, c.onBroadcast)

	c.router.Register(&protoGateway.LoginReq{}, c.onLogin)
	c.router.Register(&protoGateway.RegisterReq{}, c.onRegister)
	c.router.Register(&protoGateway.GetServerListReq{}, c.onGetServerList)
}

func (c *Conn) onWsMsg(ctx tree.Context, msg interface{}) {
	c.handleWsMsg(ctx, msg.(*WsMsg).Data)
}

func (c *Conn) onFrame(ctx tree.Context, msg interface{}) {
	m := msg.(*protoGateway.Frame)
	if len(m.Payload) == 0 {
		return
	}
	if err := c.conn.WriteMessage(websocket.BinaryMessage, m.Payload); err != nil {
		log.Printf("ConnActor[%d]: ws write error: %v", c.connID, err)
		ctx.Stop()
	}
}

func (c *Conn) onBroadcast(_ tree.Context, msg interface{}) {
	_ = c.conn.WriteMessage(websocket.BinaryMessage, msg.(*protoGateway.BroadcastMsg).Data)
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

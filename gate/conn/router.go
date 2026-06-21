package conn

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gorilla/websocket"
)

func initRouter(c *Actor) {
	c.router.Register(&WsMsg{}, c.onWsMsg)
	c.router.Register(&stopMsg{}, c.onStop)

	c.router.Register(&protoGateway.Frame{}, c.onFrame)
	c.router.Register(&protoGateway.BroadcastMsg{}, c.onBroadcast)

	c.router.Register(&protoGateway.LoginReq{}, c.onLogin)
	c.router.Register(&protoGateway.RegisterReq{}, c.onRegister)
}

func (c *Actor) onWsMsg(ctx actor.ActorContext, msg interface{}) {
	c.handleWsMsg(ctx, msg.(*WsMsg).Data)
}

func (c *Actor) onFrame(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*protoGateway.Frame)
	if len(m.Payload) == 0 {
		return
	}
	if err := c.conn.WriteMessage(websocket.BinaryMessage, m.Payload); err != nil {
		log.Printf("ConnActor[%d]: ws write error: %v", c.connID, err)
		ctx.Stop()
	}
}

func (c *Actor) onBroadcast(_ actor.ActorContext, msg interface{}) {
	_ = c.conn.WriteMessage(websocket.BinaryMessage, msg.(*protoGateway.BroadcastMsg).Data)
}

func (c *Actor) onStop(ctx actor.ActorContext, _ interface{}) {
	ctx.Stop()
}

package conn

import (
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gorilla/websocket"
)

// RegistryHasServer 由外部（main）注入，避免 conn ↔ registry 循环依赖。
var RegistryHasServer func(serverID string) bool

func registryHasServer(serverID string) bool {
	return RegistryHasServer != nil && RegistryHasServer(serverID)
}

func initRouter(c *Actor) {
	c.router.Register(&WsMsg{}, c.onWsMsg)
	c.router.Register(&protoGateway.Frame{}, c.onFrame)
	c.router.Register(&protoGateway.BroadcastMsg{}, c.onBroadcast)
	c.router.Register(&stopMsg{}, c.onStop)
	c.router.Register(&protoGateway.LoginReq{}, c.onLogin)
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

func (c *Actor) onLogin(ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGateway.LoginReq)
	serverID := fmt.Sprintf("%d", req.ServerId)
	if !registryHasServer(serverID) {
		log.Printf("ConnActor[%d]: login failed, server %s not available", c.connID, serverID)
		c.Reply(&protoGateway.LoginResp{Code: -1, Msg: "server not available"})
		return
	}
	c.uid = req.Uid
	c.serverID = serverID
	c.forward(ctx, req)
	log.Printf("ConnActor[%d]: uid=%d logged in -> server %s", c.connID, c.uid, c.serverID)
}

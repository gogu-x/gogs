package conn

import (
	"fmt"
	"log"
	"reflect"

	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func (c *Actor) handleWsMsg(ctx actor.Context, data []byte) {
	inner, err := c.codec.Unmarshal(data)
	if err != nil {
		log.Printf("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}
	for _, mw := range c.middlewares {
		if !mw(ctx, inner) {
			return
		}
	}
	c.router.SetFallback(func(ctx actor.Context, _ interface{}) {
		c.forward(ctx, inner)
	})
	c.router.Route(ctx, inner)
}

func (c *Actor) forward(ctx actor.Context, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		return
	}
	body, _ := c.codec.Marshal(protoMsg)
	if c.serverID == "" || c.nodeID == "" {
		return
	}
	if err := natsrpc.Cast(natsrpc.GameNats, c.serverID, c.nodeID, &protoGateway.Frame{
		ConnId:   c.connID,
		Uid:      c.uid,
		ServerId: c.serverID,
		GateId:   fmt.Sprintf("%d", config.GateID),
		Payload:  body,
		MsgType:  reflect.TypeOf(inner).Elem().Name(),
	}); err != nil {
		log.Printf("ConnActor[%d]: forward cast error: %v", c.connID, err)
	}
}

func (c *Actor) Reply(msg proto.Message) {
	data, err := c.codec.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Actor) onNodeFailover(_ actor.Context, msg interface{}) {
	m := msg.(*NodeFailoverMsg)
	if c.state != stateAuthed || c.serverID != m.ServerID || c.nodeID != m.DeadNodeID {
		return
	}
	inst, ok := cluster.HashPick(m.ServerID, c.uid)
	if !ok {
		log.Printf("ConnActor[%d]: uid=%d failover: no available node for server=%s", c.connID, c.uid, m.ServerID)
		return
	}
	c.nodeID = inst.NodeID
	log.Printf("ConnActor[%d]: uid=%d failover server=%s node %s -> %s", c.connID, c.uid, m.ServerID, m.DeadNodeID, c.nodeID)
}

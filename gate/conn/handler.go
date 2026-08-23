package conn

import (
	"fmt"
	"log"
	"reflect"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func (c *Conn) handleWsMsg(ctx actor.Context, data []byte) {
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

func (c *Conn) forward(ctx actor.Context, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		return
	}
	body, _ := c.codec.Marshal(protoMsg)

	msg := &protoGateway.Frame{
		ConnId:   c.connID,
		Uid:      c.uid,
		ServerId: c.serverID,
		GateId:   fmt.Sprintf("%d", config.GateID),
		Payload:  body,
		MsgType:  reflect.TypeOf(inner).Elem().Name(),
	}
	if c.stream == nil {
		log.Printf("ConnActor[%d]: game stream is unavailable", c.connID)
		return
	}
	if err := c.stream.Send(msg); err != nil {
		log.Printf("ConnActor[%d]: send stream error: %v", c.connID, err)
		ctx.Stop()
		return
	}
}

func (c *Conn) Reply(msg proto.Message) {
	data, err := c.codec.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Conn) onNodeFailover(_ actor.Context, msg interface{}) {

}

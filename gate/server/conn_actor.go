package server

import (
	"fmt"
	actor "github.com/gogu-x/bigTree"
	msgcodec "github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/pb/gateway"
	"log"
	"reflect"

	"google.golang.org/protobuf/proto"

	"github.com/gorilla/websocket"
)

type WsMsg struct {
	Data []byte
}

type stopMsg struct{}

func connActorName(connID uint64) string {
	return fmt.Sprintf("conn-%d", connID)
}

type ConnActor struct {
	conn     *websocket.Conn
	uid      uint64
	connID   uint64
	serverID string
	router   actor.Router
	codec    msgcodec.Codec
}

func NewConnActor(conn *websocket.Conn, codec msgcodec.Codec) *ConnActor {
	return &ConnActor{conn: conn, codec: codec}
}

func (c *ConnActor) OnInit(ctx actor.ActorContext) {
	c.connID = ctx.Self().ID
	ctx.Register(connActorName(c.connID))
	initGateRouter(c)
}

func (c *ConnActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *WsMsg:
		c.handleWsMsg(ctx, m.Data)
	case *gateway.Frame:
		if len(m.Payload) == 0 {
			return
		}
		if err := c.conn.WriteMessage(websocket.BinaryMessage, m.Payload); err != nil {
			log.Printf("ConnActor[%d]: ws write error: %v", c.connID, err)
			ctx.Stop()
		}
	case *stopMsg:
		ctx.Stop()
	}
}

func (c *ConnActor) handleWsMsg(ctx actor.ActorContext, data []byte) {
	inner, err := c.codec.Unmarshal(data)
	if err != nil {
		log.Printf("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}

	c.router.Route(ctx, inner)

	c.serverID = "1"
	c.router.SetFallback(func(ctx actor.ActorContext, msg interface{}) {
		protoMsg, ok := inner.(proto.Message)
		if !ok {
			log.Printf("ConnActor[%d]: msg %T is not proto.Message", c.connID, inner)
			return
		}
		body, _ := c.codec.Marshal(protoMsg)

		c.forward(ctx, &gateway.Frame{
			ConnId:   c.connID,
			Uid:      c.uid,
			ServerId: c.serverID,
			Payload:  body,
			MsgType:  reflect.TypeOf(inner).Elem().Name(),
		})
	})
}

func (c *ConnActor) forward(ctx actor.ActorContext, frame *gateway.Frame) {
	pid, ok := ctx.Lookup(StreamActorName(c.serverID))
	if !ok {
		log.Printf("ConnActor[%d]: stream actor not found for server %s", c.connID, c.serverID)
		return
	}
	ctx.Send(pid, &StreamMsg{Frame: frame})
}

func (c *ConnActor) Reply(msg proto.Message) {
	data, err := c.codec.Marshal(msg)
	if err != nil {
		log.Printf("ConnActor[%d]: marshal error: %v", c.connID, err)
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *ConnActor) OnStop(ctx actor.ActorContext) {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

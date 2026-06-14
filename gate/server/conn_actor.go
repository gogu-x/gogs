package server

import (
	"fmt"
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	msgcodec "github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/protoGateway"

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
	case *protoGateway.Frame:
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

	c.router.SetFallback(func(ctx actor.ActorContext, msg interface{}) {
		c.forward(ctx, inner)
	})

	c.router.Route(ctx, inner)
}

func (c *ConnActor) forward(ctx actor.ActorContext, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		log.Printf("ConnActor[%d]: msg %T is not proto.Message", c.connID, inner)
		return
	}
	body, _ := c.codec.Marshal(protoMsg)

	frame := &protoGateway.Frame{
		ConnId:   c.connID,
		Uid:      c.uid,
		ServerId: c.serverID,
		GateId:   fmt.Sprintf("%d", config.GateID),
		Payload:  body,
		MsgType:  reflect.TypeOf(inner).Elem().Name(),
	}

	if c.serverID == "" {
		log.Printf("ConnActor[%d]: no game node assigned, drop message", c.connID)
		return
	}
	ctx.Send(actor.MustLookup(ActorNats), &StreamMsg{Frame: frame})
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

package conn

import (
	"fmt"
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"google.golang.org/protobuf/proto"

	"github.com/gorilla/websocket"
)

type WsMsg struct{ Data []byte }
type stopMsg struct{}

type Actor struct {
	conn     *websocket.Conn
	uid      uint64
	connID   uint64
	serverID string
	router   actor.Router
	codec    codec.Codec
}

func New(c *websocket.Conn, cd codec.Codec) *Actor {
	return &Actor{conn: c, codec: cd}
}

func (c *Actor) OnInit(ctx actor.ActorContext) {
	c.connID = ctx.Self().ID
	ctx.Register(constant.ConnName(c.connID))
	initRouter(c)

	if pid, ok := ctx.Lookup(constant.ActorGateServer); ok {
		ctx.Send(pid, &protoGateway.ConnRegMsg{ConnId: c.connID})
	}

	self := ctx.Self()
	go func() {
		defer actor.Send(self, &stopMsg{})
		for {
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				return
			}
			actor.Send(self, &WsMsg{Data: data})
		}
	}()
}

func (c *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	c.router.Route(ctx, msg)
}

func (c *Actor) OnStop(ctx actor.ActorContext) {
	if pid, ok := ctx.Lookup(constant.ActorGateServer); ok {
		ctx.Send(pid, &protoGateway.ConnUnregMsg{ConnId: c.connID})
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Actor) handleWsMsg(ctx actor.ActorContext, data []byte) {
	inner, err := c.codec.Unmarshal(data)
	if err != nil {
		log.Printf("ConnActor[%d]: unmarshal error: %v", c.connID, err)
		return
	}
	c.router.SetFallback(func(ctx actor.ActorContext, _ interface{}) {
		c.forward(ctx, inner)
	})
	c.router.Route(ctx, inner)
}

func (c *Actor) forward(ctx actor.ActorContext, inner interface{}) {
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		log.Printf("ConnActor[%d]: not proto.Message", c.connID)
		return
	}
	body, _ := c.codec.Marshal(protoMsg)
	if c.serverID == "" {
		log.Printf("ConnActor[%d]: no game node assigned, drop", c.connID)
		return
	}
	ctx.Send(actor.MustLookup(constant.ActorNats), &natsrpc.OutboundMsg{
		Frame: &protoGateway.Frame{
			ConnId:   c.connID,
			Uid:      c.uid,
			ServerId: c.serverID,
			GateId:   fmt.Sprintf("%d", config.GateID),
			Payload:  body,
			MsgType:  reflect.TypeOf(inner).Elem().Name(),
		},
	})
}

func (c *Actor) Reply(msg proto.Message) {
	data, err := c.codec.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
}

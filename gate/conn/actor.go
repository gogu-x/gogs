package conn

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gorilla/websocket"
)

type WsMsg struct{ Data []byte }
type WriteMsg struct{ Data []byte } // inbound from NatsActor →write to ws
type stopMsg struct{}

type connState int

const (
	stateAnon    connState = iota // 未登录
	stateLogging                  // 登录/注册中
	stateAuthed                   // 已登录
)

type middlewareFunc func(actor.ActorContext, interface{}) bool

type Actor struct {
	conn        *websocket.Conn
	uid         uint64
	connID      uint64
	serverID    string
	token       string
	state       connState
	middlewares []middlewareFunc
	router      actor.Router
	codec       codec.Codec
}

func New(c *websocket.Conn, cd codec.Codec) *Actor {
	return &Actor{conn: c, codec: cd}
}

func (c *Actor) OnInit(ctx actor.ActorContext) {
	c.connID = ctx.Self().ID
	ctx.Register(constant.ConnName(c.connID))

	initRouter(c)
	c.middlewares = []middlewareFunc{c.checkAuth}

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
	if c.uid != 0 && c.serverID != "" {
		ctx.Send(actor.MustLookup(constant.ActorNats), &natsrpc.SendMsg{
			Module: natsrpc.ModuleGame,
			NodeID: c.serverID,
			Frame: &protoGateway.Frame{
				ConnId:  c.connID,
				Uid:     c.uid,
				MsgType: natsrpc.MsgTypeDisconnect,
			},
		})
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

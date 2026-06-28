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

// NodeFailoverMsg 节点故障通知，GateServer 广播给所有 ConnActor。
// ConnActor 收到后若自己的 nodeID == DeadNodeID，则重新 hash 选节点实现无感切换。
type NodeFailoverMsg struct {
	ServerID   string
	DeadNodeID string
}

type connState int

const (
	stateAnon    connState = iota // 未登录
	stateLogging                  // 登录/注册 中
	stateAuthed                   // 已登录
)

type middlewareFunc func(actor.ActorContext, interface{}) bool

type Actor struct {
	conn        *websocket.Conn
	uid         uint64
	connID      uint64
	serverID    string
	nodeID      string // hash 选定的 game 节点实例 ID，登录时确定，后续消息固定路由到此节点
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
			Module: natsrpc.GameNats,
			ID:     c.serverID,
			NodeId: c.nodeID,
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

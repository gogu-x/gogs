package conn

import (
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"github.com/gorilla/websocket"
)

type WsMsg struct{ Data []byte }
type WriteMsg struct{ Data []byte } // inbound from NatsActor →write to ws
type stopMsg struct{}

// NodeFailoverMsg 节点故障通知，GateServer 广播给所有 ConnActor。
// ConnActor 收到后若自己的 nodeID == DeadNodeID，则重新 hash 选节点实现无感切换。
type NodeFailoverMsg struct {
	ServerID   uint64
	DeadNodeID string
}

type connState int

const (
	stateAnon    connState = iota // 未登录
	stateLogging                  // 登录/注册 中
	stateAuthed                   // 已登录
)

type middlewareFunc func(tree.Context, interface{}) bool

type Conn struct {
	conn        *websocket.Conn
	uid         uint64
	connID      uint64
	serverID    uint64
	nodeID      string // hash 选定的 game 节点实例 ID，登录时确定，后续消息固定路由到此节点
	token       string
	state       connState
	middlewares []middlewareFunc
	router      tree.Router
	codec       codec.Codec
}

func New(connID uint64, c *websocket.Conn, cd codec.Codec) *Conn {
	return &Conn{connID: connID, conn: c, codec: cd}
}

func (c *Conn) Name() string { return constant.ConnName(c.connID) }

// MailboxSize 连接型 Conn 消息速率低，用小 mailbox 省内存：
// 2 万连接下 64 与默认 128 相差约 100MB 量级的 envelope 槽位。
func (c *Conn) MailboxSize() int { return 64 }

func (c *Conn) OnInit(ctx tree.Context) {
	initRouter(c)
	c.middlewares = []middlewareFunc{c.checkAuth}

	if pid, ok := ctx.Lookup(constant.ActorGateServer); ok {
		ctx.Send(pid, &protoGateway.ConnRegMsg{ConnId: c.connID})
	}

	self := ctx.Self()
	go func() {
		defer tree.Send(self, &stopMsg{})
		for {
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				return
			}
			tree.Send(self, &WsMsg{Data: data})
		}
	}()
}

func (c *Conn) HandleMessage(ctx tree.Context, msg interface{}) {
	c.router.Route(ctx, msg)
}

func (c *Conn) OnStop(ctx tree.Context) {
	if pid, ok := ctx.Lookup(constant.ActorGateServer); ok {
		ctx.Send(pid, &protoGateway.ConnUnregMsg{ConnId: c.connID})
	}
	if c.uid != 0 && c.serverID != 0 {

	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

package conn

import (
	"reflect"

	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
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
	StateAnon    connState = iota // 未登录
	StateLoggIng                  // 登录 中
	StateRegIng                   // 注册中
	StateAuthed                   // 已登录
)

type hookHandler func(tree.Context, interface{})

type Conn struct {
	conn     *websocket.Conn
	uid      uint64
	connID   uint64
	serverID uint32
	nodeID   string
	token    string
	state    connState
	hook     map[reflect.Type]hookHandler
	stream   pb_gateway.Gateway_StreamClient
	grpcConn *grpc.ClientConn
	router   tree.Router
	codec    codec.Codec
}

func New(connID uint64, c *websocket.Conn, cd codec.Codec) *Conn {
	return &Conn{connID: connID, conn: c, codec: cd, hook: make(map[reflect.Type]hookHandler)}
}

func (c *Conn) Name() string { return constant.ConnName(c.connID) }

// MailboxSize 连接型 Conn 消息速率低，用小 mailbox 省内存：
// 2 万连接下 64 与默认 128 相差约 100MB 量级的 envelope 槽位。
func (c *Conn) MailboxSize() int { return 64 }

func (c *Conn) OnInit(ctx tree.Context) {
	initHandler(c)
	initHook(c)

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
		ctx.Send(pid, &pb_gateway.ConnUnregMsg{ConnId: c.connID})
	}
	if c.uid != 0 && c.serverID != 0 {

	}
	if c.stream != nil {
		_ = c.stream.CloseSend()
		c.stream = nil
	}
	if c.grpcConn != nil {
		_ = c.grpcConn.Close()
		c.grpcConn = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

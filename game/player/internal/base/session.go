package base

import (
	"log"
	"reflect"
	"time"

	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// Session 每次请求携带的网络上下文
type Session struct {
	ConnID       uint64
	GateId       string
	Data         *PlayerData
	ctx          tree.Context
	currentFrame *protoGateway.Frame // 当前正在处理的 Frame，Reply 据此判断回包路径
}

func NewSession(data *PlayerData, ctx tree.Context) *Session {
	return &Session{Data: data, ctx: ctx}
}

// SetCurrentFrame 由框架在处理 Frame 前后调用，业务层无需感知
func (s *Session) SetCurrentFrame(f *protoGateway.Frame) {
	s.currentFrame = f
}
func (s *Session) AfterFunc(d time.Duration, cb func()) {
	s.ctx.AfterFunc(d, func(_ tree.Context) { cb() })
}

// Reply 将回包通过 NATS 发回 Gate
func (s *Session) Reply(msg proto.Message) {
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("Reply marshal error: %v", err)
		return
	}
	if err := natsrpc.Cast(natsrpc.ModuleGate, s.GateId, "", &protoGateway.Frame{
		Uid:     s.Data.UID,
		ConnId:  s.ConnID,
		GateId:  s.GateId,
		Payload: body,
		MsgType: reflect.TypeOf(msg).Elem().Name(),
	}); err != nil {
		log.Printf("Reply cast error: %v", err)
	}
}

// Request 向目标 Actor 发请求，回调在本 Actor goroutine 内执行
func (s *Session) Request(pid tree.PID, msg interface{}, cb func(interface{}, error)) {
	s.ctx.Request(pid, msg).Callback(s.ctx, cb)
}

// Handle 将 ctl 层函数包装为 actor.Handler
func (s *Session) Handle(fn func(*Session, interface{})) tree.Handler {
	return func(ctx tree.Context, msg interface{}) {
		fn(s, msg)
	}
}

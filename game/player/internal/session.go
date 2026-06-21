package internal

import (
	"log"
	"reflect"
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"google.golang.org/protobuf/proto"
)

// Session 每次请求携带的网络上下文
type Session struct {
	ConnID uint64
	GateId string
	Data   *PlayerData
	ctx    actor.ActorContext
}

func NewSession(data *PlayerData, ctx actor.ActorContext) *Session {
	return &Session{Data: data, ctx: ctx}
}

// AfterFunc 在 Actor goroutine 内调度一个定时回调，可在任意地方调用
func (s *Session) AfterFunc(d time.Duration, cb func()) {
	s.ctx.AfterFunc(d, func(_ actor.ActorContext) { cb() })
}

// Reply 将回包通过 NATS 发回 Gate
func (s *Session) Reply(msg proto.Message) {
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("Reply marshal error: %v", err)
		return
	}
	actor.Send(actor.MustLookup(constant.ActorNats), &natsrpc.SendMsg{
		Module: natsrpc.ModuleGate,
		NodeID: s.GateId,
		Frame: &protoGateway.Frame{
			Uid:     s.Data.UID,
			ConnId:  s.ConnID,
			GateId:  s.GateId,
			Payload: body,
			MsgType: reflect.TypeOf(msg).Elem().Name(),
		},
	})
}

// Request 向目标 Actor 发请求，回调在本 Actor goroutine 内执行
func (s *Session) Request(pid actor.PID, msg interface{}, cb func(interface{}, error)) {
	s.ctx.Request(pid, msg).Callback(s.ctx, cb)
}

// Handle 将 ctl 层函数包装为 actor.Handler
func (s *Session) Handle(fn func(*Session, interface{})) actor.Handler {
	return func(ctx actor.ActorContext, msg interface{}) {
		fn(s, msg)
	}
}

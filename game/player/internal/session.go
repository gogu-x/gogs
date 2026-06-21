package internal

import (
	"log"
	"reflect"

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
}

func NewSession(data *PlayerData) *Session {
	return &Session{Data: data}
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

// Handle 将 ctl 层函数包装为 actor.Handler
func (s *Session) Handle(fn func(*Session, actor.ActorContext, interface{})) actor.Handler {
	return func(ctx actor.ActorContext, msg interface{}) {
		fn(s, ctx, msg)
	}
}

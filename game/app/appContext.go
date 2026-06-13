package app

import (
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"google.golang.org/protobuf/proto"
)

// NatsReplier 由 model 包注入，避免 import cycle
var NatsReplier func(connID uint64, payload []byte) error

// Reply 将回包通过 NATS 发回 Gate
func (a *App) Reply(msg proto.Message) {
	if NatsReplier == nil {
		log.Printf("Reply: NatsReplier not set")
		return
	}
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		log.Printf("Reply marshal error: %v", err)
		return
	}
	frame := &protoGateway.Frame{
		Uid:     a.Player.UID,
		ConnId:  a.ConnID,
		Payload: body,
		MsgType: reflect.TypeOf(msg).Elem().Name(),
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		log.Printf("Reply frame marshal error: %v", err)
		return
	}
	if err := NatsReplier(a.ConnID, data); err != nil {
		log.Printf("Reply error: %v", err)
	}
}

// Handle 将业务函数包装为 actor.Handler，ctx 直接传 actor.ActorContext
func (a *App) Handle(fn func(*App, actor.ActorContext, interface{})) actor.Handler {
	return func(ctx actor.ActorContext, msg interface{}) {
		fn(a, ctx, msg)
	}
}

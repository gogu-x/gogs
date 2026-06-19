package natsrpc

import (
	"fmt"
	"log"
	"reflect"

	actor "github.com/gogu-x/bigTree"
	"google.golang.org/protobuf/proto"
)

// Meta 携带 NATS 消息的路由元信息。
type Meta struct {
	UID    uint64
	ConnID uint64
	GateID string
}

var (
	protoRegistry = map[reflect.Type]func(actor.ActorContext, proto.Message, Meta){}
	protoFallback func(actor.ActorContext, proto.Message, Meta)
)

// Register 注册 proto 消息类型的处理函数。
func Register(prototype proto.Message, fn func(actor.ActorContext, proto.Message, Meta)) {
	t := reflect.TypeOf(prototype)
	if _, dup := protoRegistry[t]; dup {
		panic(fmt.Sprintf("natsrpc: handler already registered for %v", t))
	}
	protoRegistry[t] = fn
}

// SetFallback 注册兜底 handler，处理未精确匹配类型的 proto 消息。
func SetFallback(fn func(actor.ActorContext, proto.Message, Meta)) {
	protoFallback = fn
}

func dispatchProto(ctx actor.ActorContext, msg proto.Message, meta Meta) {
	fn, ok := protoRegistry[reflect.TypeOf(msg)]
	if !ok {
		fn = protoFallback
	}
	if fn == nil {
		log.Printf("natsrpc: no handler for %T", msg)
		return
	}
	fn(ctx, msg, meta)
}

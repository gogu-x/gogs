package model

import (
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	natsclient "github.com/gogu-x/gogs/nats"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// NatsActor 订阅 NATS gate.in.{serverID}，将消息路由到对应 PlayerActor
type NatsActor struct {
	sub *nats.Subscription
}

func (a *NatsActor) OnInit(ctx actor.ActorContext) {
	serverID := fmt.Sprintf("%d", config.ServerID)
	sub, err := natsclient.SubscribeGameIn(serverID, func(data []byte) {
		var frame protoGateway.Frame
		if err := proto.Unmarshal(data, &frame); err != nil {
			log.Printf("NatsActor: unmarshal frame error: %v", err)
			return
		}
		if len(frame.Payload) == 0 {
			return
		}
		inner, err := codec.ProtoCodec.Unmarshal(frame.Payload)
		if err != nil {
			log.Printf("NatsActor: unmarshal payload error: %v", err)
			return
		}
		protoMsg, ok := inner.(proto.Message)
		if !ok {
			log.Printf("NatsActor: payload is not proto.Message")
			return
		}
		actor.Send(ctx.Self(), &inboundMsg{msg: protoMsg, uid: frame.Uid, connID: frame.ConnId})
	})
	if err != nil {
		log.Fatalf("NatsActor: subscribe error: %v", err)
	}
	a.sub = sub
	log.Printf("NatsActor: subscribed gate.in.%s", serverID)
}

func (a *NatsActor) OnStop(_ actor.ActorContext) {
	if a.sub != nil {
		a.sub.Unsubscribe()
	}
}

func (a *NatsActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *inboundMsg:
		name := playerActorName(m.uid)
		pid, exists := ctx.Lookup(name)
		if !exists {
			pid = actor.Spawn(name, newPlayerActor(m.uid, m.connID))
		}
		ctx.Send(pid, m)
	case []byte:
		var frame protoGateway.Frame
		if err := proto.Unmarshal(m, &frame); err != nil {
			log.Printf("NatsActor: reply unmarshal error: %v", err)
			return
		}
		if err := natsclient.PublishRawToGate(frame.ConnId, m); err != nil {
			log.Printf("NatsActor: publish error: %v", err)
		}
	}
}

package server

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	natsclient "github.com/gogu-x/gogs/nats"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"google.golang.org/protobuf/proto"
)

type NatsActor struct{}

func NewNatsActor() *NatsActor { return &NatsActor{} }

func (n *NatsActor) OnInit(_ actor.ActorContext) {
	natsclient.SubscribeGateOut(func(connID uint64, data []byte) {
		var frame protoGateway.Frame
		if err := proto.Unmarshal(data, &frame); err != nil {
			log.Printf("NatsActor: unmarshal frame error: %v", err)
			return
		}
		pid, ok := actor.Lookup(connActorName(connID))
		if !ok {
			return
		}
		actor.Send(pid, &frame)
	})
	log.Println("NatsActor: ready")
}

func (n *NatsActor) HandleMessage(_ actor.ActorContext, msg interface{}) {
	m, ok := msg.(*StreamMsg)
	if !ok {
		return
	}
	if err := natsclient.PublishToGame(m.Frame.ServerId, m.Frame); err != nil {
		log.Printf("NatsActor: publish error: %v", err)
	}
}

func (n *NatsActor) OnStop(_ actor.ActorContext) {}

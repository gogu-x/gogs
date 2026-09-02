package nats

import (
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/natsrpc"
)

func NewActor() *natsrpc.Actor {
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GateSubject(uint32(conf.GateID), 1), "", 20),
		},
	})
}

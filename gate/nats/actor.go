package nats

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/natsrpc"
)

func NewActor() *natsrpc.Actor {
	gateID := fmt.Sprintf("%d", config.GateID)
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GateOutSubject(gateID), func(frame *natsrpc.Frame) (actor.PID, bool) {
				return actor.Default().Lookup(constant.ConnName(frame.ConnId))
			}, 20),
		},
	})
}

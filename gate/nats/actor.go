package nats

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/natsrpc"
)

func NewActor() *natsrpc.Actor {
	return natsrpc.NewActor(natsrpc.ActorConfig{
		GateOut: fmt.Sprintf("%d", config.GateID),
		LookupConn: func(connID uint64) (actor.PID, bool) {
			return actor.Lookup(constant.ConnName(connID))
		},
	})
}

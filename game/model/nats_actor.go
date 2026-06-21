package model

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/natsrpc"
)

func NewNatsActor(instID string) *natsrpc.Actor {
	serverID := fmt.Sprintf("%d", config.ServerID)
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GameInSubject(serverID), func(frame *natsrpc.Frame) (actor.PID, bool) {
				name := constant.PlayerName(frame.Uid)
				pid, exists := actor.Default().Lookup(name)
				if !exists {
					pid = actor.Default().Spawn(name, newPlayerActor(frame.Uid, frame.ConnId))
				}
				return pid, true
			}, 1),
			natsrpc.ShutdownSub(serverID, instID),
		},
	})
}

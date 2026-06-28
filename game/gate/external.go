package gate

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/gate/internal"
	"github.com/gogu-x/gogs/game/player"
	"github.com/gogu-x/gogs/natsrpc"
)

// NewGateActor 创建 GateActor，负责 gRPC 服务和集群注册
func NewGateActor() actor.Actor {
	return internal.NewGateActor()
}

// NewNatsActor Gate 创建 NatsActor，负责接收消息并 spawn PlayerActor
func NewNatsActor(NodeID string) *natsrpc.Actor {
	serverID := fmt.Sprintf("%d", config.ServerID)
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GameInSubject(serverID, NodeID), func(frame *natsrpc.Frame) (actor.PID, bool) {
				name := constant.PlayerName(frame.Uid)
				pid, exists := actor.Default().Lookup(name)
				if !exists {
					pid = actor.Default().Spawn(name, player.NewPlayerActor(frame.Uid, frame.ConnId))
				}
				return pid, true
			}, 1),
			natsrpc.ShutdownSub(serverID, NodeID),
		},
	})
}

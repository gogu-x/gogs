package gate

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/gate/internal"
)

// NewGateActor 创建 GateActor，负责 gRPC 服务和集群注册
func NewGateActor() actor.Actor {
	return internal.NewGateActor()
}

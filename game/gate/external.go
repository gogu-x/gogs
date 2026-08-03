package gate

import (
	"github.com/gogu-x/gogs/game/gate/internal"
	"github.com/gogu-x/tree"
)

// NewGateActor 创建 GateActor，负责 gRPC 服务和集群注册
func NewGateActor() tree.Actor {
	return internal.NewGateActor()
}

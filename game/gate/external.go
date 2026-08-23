package gate

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/gate/internal"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
)

// NewGateActor 创建 GateActor，负责 gRPC 服务和集群注册。
func NewGateActor() tree.Actor {
	return internal.NewGateActor()
}

// PushToConn routes a Game-side frame to the currently active gateway stream
// for GateID and ConnID. It is safe to call from any Game actor.
func PushToConn(gateID string, connID uint64, frame *protoGateway.Frame) bool {
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.PushToConn{GateID: gateID, ConnID: connID, Frame: frame})
}

// BanUID disconnects all current sessions for uid and rejects future frames
// after their identity is bound to that uid.
func BanUID(uid uint64) bool {
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.BanUID{UID: uid})
}

func UnbanUID(uid uint64) bool {
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.UnbanUID{UID: uid})
}

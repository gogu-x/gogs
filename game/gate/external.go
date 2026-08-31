package gate

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/gate/internal"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

// NewGate 创建 GateActor，负责 gRPC 服务和集群注册。
func NewGate() tree.Actor {
	return internal.NewGateActor()
}

// PushToConn routes a protobuf message to the active stream identified by UID.
func PushToConn(uid uint64, msg proto.Message) bool {
	if uid == 0 || msg == nil {
		return false
	}
	payload, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		return false
	}
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.PushToMsg{
		UID:   uid,
		Frame: &pb_gateway.Frame{Payload: payload},
	})
}

// BanUID disconnects the current UID session and rejects future sessions.
func BanUID(uid uint64) bool {
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.BanUID{UID: uid})
}

// UnbanUID allows a previously banned UID to open a new stream.
func UnbanUID(uid uint64) bool {
	pid, ok := tree.Lookup(constant.Gate)
	if !ok {
		return false
	}
	return tree.Send(pid, &internal.UnbanUID{UID: uid})
}

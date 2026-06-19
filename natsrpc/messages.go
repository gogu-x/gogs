package natsrpc

import (
	"google.golang.org/protobuf/proto"

	"github.com/gogu-x/gogs/pb/protoGateway"
)

// InboundMsg 经解码路由后投递给业务 Actor（如 PlayerActor）的消息。
type InboundMsg struct {
	Msg    proto.Message
	UID    uint64
	ConnID uint64
	GateID string
}

// OutboundMsg gate 侧 ConnActor 发给 NatsActor 以转发到 Game 的消息。
type OutboundMsg struct{ Frame *protoGateway.Frame }

// ReplyMsg game 侧业务 Actor 发给 NatsActor 以回包给 Gate 的消息。
type ReplyMsg struct{ Data []byte }

// rawMsg NATS goroutine 解出的原始帧，Gate NatsActor 内部使用。
type rawMsg struct {
	connID uint64
	data   []byte
}

// shutdownMsg shutdown 订阅触发。
type shutdownMsg struct{}

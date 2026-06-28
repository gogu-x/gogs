package natsrpc

import (
	"time"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

// Frame 统一的消息帧格式。
type Frame = protoGateway.Frame

// SendMsg 统一的 NATS 发送消息。
type SendMsg struct {
	Module string
	ID     string
	NodeId string
	Frame  *Frame
}

// RequestMsg 跨节点 request-reply，通过 NatsActor 中转。
// NatsActor 内部生成 RequestId、维护 pending map、处理超时，业务层无需感知。
// Callback 在发起方 Actor goroutine 内执行，第一个参数为回包的 Frame.Payload（[]byte）。
type RequestMsg struct {
	Module    string
	ID        string
	NodeId    string
	Frame     *Frame
	Timeout   time.Duration
	CallerPID actor.PID
	Callback  func(interface{}, error)
}

// shutdownMsg 关闭信号。
type shutdownMsg struct{}

type subKind int

const (
	kindSub subKind = iota
	kindShutdown
)

// SubConfig 描述一个 NATS 订阅。
type SubConfig struct {
	kind    subKind
	subject string
	workers int
	route   RouteFunc
}

// Sub 通用订阅：收到 Frame 后通过 route 找到目标 Actor 投递。
func Sub(subject string, route RouteFunc, workers int) SubConfig {
	return SubConfig{kind: kindSub, subject: subject, route: route, workers: workers}
}

// ShutdownSub 订阅关闭信号。
func ShutdownSub(serverID, NodeID string) SubConfig {
	return SubConfig{kind: kindShutdown, subject: "game.shutdown." + serverID + "." + NodeID}
}

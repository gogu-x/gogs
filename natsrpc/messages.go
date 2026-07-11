package natsrpc

import (
	"time"

	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"
)

// Frame 统一的消息帧格式。
type Frame = protoGateway.Frame

// castMsg 投递消息（fire-and-forget），投给 NatsActor 处理。
type castMsg struct {
	Module string
	ID     string
	NodeId string
	Frame  *Frame
}

// callMsg 异步超时请求，通过 NatsActor 中转。
// NatsActor 内部生成 RequestId（NATS reply inbox subject）、维护 pending map、处理超时，
// 业务层无需感知底层细节，只需拿到 Callback(frame, err)。
// Callback 在发起方 Actor goroutine 内执行。
type callMsg struct {
	Module    string
	ID        string
	NodeId    string
	Frame     *Frame
	Timeout   time.Duration
	CallerPID actor.PID
	Callback  func(*Frame, error)
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

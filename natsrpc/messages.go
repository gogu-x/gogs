package natsrpc

import "github.com/gogu-x/gogs/pb/protoGateway"

// Frame 统一的消息帧格式。
type Frame = protoGateway.Frame

// SendMsg 统一的 NATS 发送消息。
// Module 指定目标类型，NodeID 指定目标实例 ID。
type SendMsg struct {
	Module string
	NodeID string
	Frame  *Frame
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
func ShutdownSub(serverID, instID string) SubConfig {
	return SubConfig{kind: kindShutdown, subject: "game.shutdown." + serverID + "." + instID}
}

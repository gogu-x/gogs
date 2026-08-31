package natsrpc

import (
	"time"

	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// castMsg 投递消息（fire-and-forget），投给 NatsActor 处理。
type castMsg struct {
	Module string
	ID     string
	NodeId string
	Msg    []byte
}

// callMsg 异步超时请求，通过 NatsActor 中转。
// Callback 在发起方 Actor goroutine 内执行。
type callMsg struct {
	Module    string
	ID        string
	NodeId    string
	Msg       []byte
	Timeout   time.Duration
	CallerPID tree.PID
	Callback  func(proto.Message, error)
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
	Pid     tree.PID
}

// Sub 通用订阅：收到 Frame 后通过 route 找到目标 Actor 投递。
func Sub(subject string, actorName string, workers int) SubConfig {
	return SubConfig{kind: kindSub, subject: subject, workers: workers}
}

// ShutdownSub 订阅关闭信号。
func ShutdownSub(serverID, NodeID string) SubConfig {
	return SubConfig{kind: kindShutdown, subject: "game.shutdown." + serverID + "." + NodeID}
}

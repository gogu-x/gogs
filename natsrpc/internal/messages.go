package internal

import (
	"fmt"
	"time"

	"github.com/gogu-x/tree"
)

const subIn = "sub:%v:%v:%v"

func Subject(moduleType string, serverID, NodeID int) string {
	return fmt.Sprintf(subIn, moduleType, serverID, NodeID)
}

// NatsMsg 异步超时请求，通过 NatsActor 中转。
type NatsMsg struct {
	SendModule   string
	TaggerModule string
	ID           int
	NodeId       int
	Msg          interface{}
	Timeout      time.Duration
	SendName     string
	TaggerName   string
	Callback     func(tree.Context, interface{}, error)

	// Cast marks a fire-and-forget notification. It is kept on the internal
	// envelope so Cast and Call can share the Nats actor mailbox safely.
	Cast bool
}

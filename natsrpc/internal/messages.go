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

// CastMsg 投递消息（fire-and-forget），投给 NatsActor 处理。
type CastMsg struct {
	Module string
	ID     int
	NodeId int
	Msg    interface{}
}

// CallMsg 异步超时请求，通过 NatsActor 中转。
type CallMsg struct {
	SendModule   string
	TaggerModule string
	ID           int
	NodeId       int
	Msg          interface{}
	Timeout      time.Duration
	SendName     string
	TaggerName   string
	Callback     func(tree.Context, interface{}, error)
}

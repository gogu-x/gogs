package server

import "github.com/gogu-x/gogs/pb/protoGateway"

// SwitchMsg 通知 NatsActor 切换目标 serverID
type SwitchMsg struct {
	ServerID string
	NewAddr  string
}

// StreamMsg 包装发往 game 的消息帧
type StreamMsg struct {
	Frame *protoGateway.Frame
}

// checkDeleteMsg 延迟确认 active 实例是否真的下线
type checkDeleteMsg struct {
	ServerID string
	InstID   string
}

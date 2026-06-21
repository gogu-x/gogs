package natsrpc

// Module 标识目标 NATS 模块类型。
const (
	ModuleGame    = "game"
	ModuleGate    = "gate"
	ModuleCross   = "cross"
	ModuleDeliver = "deliver"
	ModulePl      = "pl"
)

// MsgTypeDisconnect 客户端断线通知，通过 Frame.MsgType 传递给 game 侧。
const MsgTypeDisconnect = "__disconnect__"

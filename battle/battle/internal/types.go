package internal

import "google.golang.org/protobuf/proto"

// Source 标识一场战斗结果应回投的发起方 Game 节点。
type Source struct {
	ServerID int
	NodeID   int
}

// Notifier 把生命周期通知（Created/Action/Finished）投递给发起方节点。
// 由装配层提供具体实现（如走 NATS 回投），battle 只依赖该窄接口。
type Notifier interface {
	Notify(Source, proto.Message) error
}

type NotifyFunc func(Source, proto.Message) error

func (f NotifyFunc) Notify(source Source, msg proto.Message) error { return f(source, msg) }

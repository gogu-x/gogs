package server

// Actor 名称常量，用于 ActorSystem Spawn/Lookup，避免魔法字符串
const (
	// ActorNats NATS 消息总线 Actor，负责 Gate↔Game 消息的 pub/sub 转发
	ActorNats = "nats"

	// ActorRegistry 服务注册发现 Actor，Watch etcd 维护在线 Game 节点列表，提供轮询 Pick
	ActorRegistry = "registry"
)

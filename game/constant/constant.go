package constant

// Actor 名称常量，用于 ActorSystem Spawn/Lookup，避免魔法字符串
const (
	// ActorSupervisor 玩家监督者 Actor，负责管理所有 PlayerActor 生命周期
	ActorSupervisor = "supervisor"

	// ActorGame 兼容旧代码，指向 supervisor
	ActorGame = ActorSupervisor

	// ActorGuild 工会 Actor，全局唯一，持有所有工会数据
	ActorGuild = "guild"

	// ActorActivity 活动 Actor，全局唯一，管理所有活动及玩家进度
	ActorActivity = "activity"

	// ActorGate 旧 gRPC 网关 Actor，新架构已由 NATS 替代，保留供旧代码引用
	ActorGate = "gate"
)

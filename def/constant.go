package def

// Actor 名称常量，用于 ActorSystem Spawn/Lookup，避免魔法字符串
const (
	//PLAY 游戏模块
	PLAY = "PLAY"

	// CROSS 跨服模块
	CROSS = "CROSS"

	PF = "PF"

	Nats = "NATS"

	// Mongo 进程的 MongoDB actor
	Mongo = "Mongo"

	// Guild 工会 Actor，全局唯一，持有所有工会数据
	Guild = "Guild"

	// ActorActivity 活动 Actor，全局唯一，管理所有活动及玩家进度
	ActorActivity = "Activity"

	// Gate 旧 gRPC 网关 Actor，新架构已由 NATS 替代，保留供旧代码引用
	Gate = "Gate"

	Web = "Web"
)

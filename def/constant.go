package def

// Actor 名称常量，用于 ActorSystem Spawn/Lookup，避免魔法字符串
const (
	//PLAY 游戏模块
	PLAY = "PLAY"

	// CROSS 跨服模块
	CROSS = "CROSS"

	PF = "PF"

	Nats = "NATS"

	// Mongo 进程的 MongoDB battle
	Mongo = "Mongo"

	// Guild 工会 Actor，全局唯一，持有所有工会数据
	Guild = "Guild"

	// ActorActivity 活动 Actor，全局唯一，管理所有活动及玩家进度
	ActorActivity = "ACTIVITY"

	// GameGate 旧 gRPC 网关 Actor，新架构已由 NATS 替代，保留供旧代码引用
	GameGate = "GAMEGATE"

	// BattleService 是独立 Battle 进程内的服务 Actor（manager）。
	BattleService = "BATTLE_SERVICE"

	// BattleClient 是 Game 进程内接收 Battle 生命周期事件的 Actor。
	BattleClient = "BATTLE_CLIENT"

	// ServiceBattle 是 etcd 通用服务发现中的 Battle 服务类型。
	ServiceBattle = "Battle"

	// BattleActorPrefix 是运行期每个 per-battle battle 注册名的前缀。
	// 完整注册名由 BattleActorName(battleID) 生成，Game 与 Battle 两端用同一
	// 拼法派生，作为 NATS 消息的 TaggerName 直达对应战斗 battle。
	BattleActorPrefix = "BATTLE_ACTOR_"

	Web = "Web"
)

// BattleActorName 返回某场战斗的 per-battle battle 注册名（跨进程 NATS 寻址目标）。
func BattleActorName(battleID string) string { return BattleActorPrefix + battleID }

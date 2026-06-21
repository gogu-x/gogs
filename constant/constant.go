package constant

import "fmt"

// ─── RPC Actors（客户端，各进程按需 Spawn）────────────────────────────────────
const ActorRpcPlatform = "rpc_platform"

const ActorNats = "nats"

const ActorGameMongo = "game_mongo" // game 进程的 MongoDB actor

// ─── Platform Actors ──────────────────────────────────────────────────────────
const (
	ActorPlatformMongo   = "platform_mongo"
	ActorPlatformGrpc    = "platform_grpc"
	ActorPlatformWebhook = "platform_webhook"
)

// Actor 名称常量，用于 ActorSystem Spawn/Lookup，避免魔法字符串
const (

	// ActorGuild 工会 Actor，全局唯一，持有所有工会数据
	ActorGuild = "guild"

	// ActorActivity 活动 Actor，全局唯一，管理所有活动及玩家进度
	ActorActivity = "activity"

	// ActorGate 旧 gRPC 网关 Actor，新架构已由 NATS 替代，保留供旧代码引用
	ActorGate = "gate"
)

func PlayerName(uid uint64) string { return fmt.Sprintf("player-%d", uid) }

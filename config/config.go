package config

import (
	"fmt"
	"os"
	"strings"
)

var (
	// EtcdEndpoints etcd 地址，支持环境变量 ETCD_ENDPOINTS 覆盖（逗号分隔）
	EtcdEndpoints = envSlice("ETCD_ENDPOINTS", "43.160.212.55:2379")

	// GateBasePort gate WebSocket 基础端口，实际端口 = GateBasePort + GateID
	GateBasePort = 8080

	// GameBasePort game gRPC 基础端口，实际端口 = GameBasePort + ServerID
	GameBasePort = 9000

	// LogLevel 日志级别
	LogLevel = env("LOG_LEVEL", "debug")

	// GateID ，由启动参数指定，每个 gate 进程唯一
	GateID = 0

	// ServerID 区服 ID，由启动参数指定，每个 game 进程唯一
	ServerID = 0

	// NodeId ，由启动参数指定，相同game 多节点
	NodeId = 0

	// GrpcHost game 进程对外暴露的 host（容器内需设为容器名或IP）
	GrpcHost = env("GRPC_HOST", "127.0.0.1")

	// GrpcPortOverride 由启动参数 --port 覆盖，同一 serverID 多实例时用于区分端口
	GrpcPortOverride = ""

	// MongoURL MongoDB 连接地址
	MongoURL = env("MONGO_URL", "mongodb://localhost:27017")

	// NatsURL NATS 连接地址
	NatsURL = env("NATS_URL", "nats://43.160.212.55:4222")

	// PlatformAddr 平台服 gRPC 监听地址
	PlatformAddr = env("PLATFORM_ADDR", ":7000")

	// PlatformGrpcAddr 平台服 gRPC 连接地址（gate/game 侧使用）
	PlatformGrpcAddr = env("PLATFORM_GRPC_ADDR", "127.0.0.1:7000")

	// PlatformWebhookAddr 平台服 HTTP webhook 监听地址
	PlatformWebhookAddr = env("PLATFORM_WEBHOOK_ADDR", ":7001")

	// JWTSecret JWT 签名密钥
	JWTSecret = env("JWT_SECRET", "changeme-secret")
)

func GateAddr() string {
	return fmt.Sprintf(":%d", GateBasePort+GateID)
}

func GameAddr() string {
	if GrpcPortOverride != "" {
		return fmt.Sprintf("%s:%s", GrpcHost, GrpcPortOverride)
	}
	return fmt.Sprintf("%s:%d", GrpcHost, GameBasePort+ServerID)
}

func GrpcAddrFor(serverID int) string {
	return fmt.Sprintf("%s:%d", GrpcHost, GameBasePort+serverID)
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envSlice(key, defaultVal string) []string {
	v := env(key, defaultVal)
	return strings.Split(v, ",")
}

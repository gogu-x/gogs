package conf

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
)

var (
	// EtcdEndpoints etcd 地址（逗号分隔的命令行参数或 JSON 数组配置）
	EtcdEndpoints = []string{"43.160.212.55:2379"}

	// GateBasePort gate WebSocket 基础端口，实际端口 = GateBasePort + GateID
	GateBasePort = 8080

	// GameBasePort game gRPC 基础端口，实际端口 = GameBasePort + ServerID
	GameBasePort = 9900

	// LogLevel 日志级别
	LogLevel = "debug"

	// GateID ，由启动参数指定，每个 gate 进程唯一
	GateID = 1

	// ServerID 区服 ID，由启动参数指定，每个 game 进程唯一
	ServerID = 1

	// NodeId ，由启动参数指定，相同game 多节点
	NodeId = 1

	// GrpcHost game 进程对外暴露的 host（容器内需设为容器名或IP）
	GrpcHost = "127.0.0.1"

	// GrpcPortOverride 由启动参数 --port 覆盖，同一 serverID 多实例时用于区分端口。
	GrpcPortOverride = ""

	// MongoURL MongoDB 连接地址
	MongoURL = "mongodb://43.160.212.55:27017"

	// MongoUsername MongoDB 认证用户名
	MongoUsername = ""

	// MongoPassword MongoDB 认证密码
	MongoPassword = ""

	// NatsURL NATS 连接地址
	NatsURL = "nats://43.160.212.55:4222"

	// PlatformAddr 平台服 gRPC 监听地址
	PlatformAddr = ":7000"

	// PlatformGrpcAddr 平台服 gRPC 连接地址（gate/game 侧使用）
	PlatformGrpcAddr = "127.0.0.1:7000"

	// PlatformWebhookAddr 平台服 HTTP webhook 监听地址
	PlatformWebhookAddr = ":7001"

	// JWTSecret JWT 签名密钥
	JWTSecret = "changeme-secret"

	LogPath = ""
)

// ConnectionFlags returns flags shared by all service entrypoints. Values passed
// explicitly on the command line override values from --config.
func ConnectionFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{Name: "server-id", Usage: "server-id"},
		&cli.IntFlag{Name: "gate-id", Usage: "gate-id"},
		&cli.IntFlag{Name: "node-id", Usage: "node-id"},
		&cli.StringFlag{Name: "config", Usage: "JSON configuration file"},
		&cli.StringFlag{Name: "etcd", Usage: "etcd, comma-separated"},
		&cli.IntFlag{Name: "gate-base-port", Usage: "gate WebSocket base port"},
		&cli.IntFlag{Name: "game-base-port", Usage: "game gRPC base port"},
		&cli.StringFlag{Name: "grpc-host", Usage: "game gRPC host"},
		&cli.StringFlag{Name: "mongo-url", Usage: "MongoDB connection URI"},
		&cli.StringFlag{Name: "mongo-username", Usage: "MongoDB authentication username"},
		&cli.StringFlag{Name: "mongo-password", Usage: "MongoDB authentication password"},
		&cli.StringFlag{Name: "nats-url", Usage: "NATS connection URI"},
		&cli.StringFlag{Name: "platform-addr", Usage: "platform gRPC listen address"},
		&cli.StringFlag{Name: "platform-grpc-addr", Usage: "platform gRPC connection address"},
		&cli.StringFlag{Name: "platform-webhook-addr", Usage: "platform webhook listen address"},
		&cli.StringFlag{Name: "log_path", Usage: "log path"},
	}
}

// LoadAndApply loads --config first, then applies only explicitly supplied
// command-line connection settings, giving the command line precedence.
func LoadAndApply(c *cli.Command) error {

	if c.IsSet("etcd") {
		EtcdEndpoints = splitEndpoints(c.String("etcd"))
	}
	if c.IsSet("gate-base-port") {
		GateBasePort = c.Int("gate-base-port")
	}
	if c.IsSet("game-base-port") {
		GameBasePort = c.Int("game-base-port")
	}
	if c.IsSet("grpc-host") {
		GrpcHost = c.String("grpc-host")
	}
	if c.IsSet("mongo-url") {
		MongoURL = c.String("mongo-url")
	}
	if c.IsSet("mongo-username") {
		MongoUsername = c.String("mongo-username")
	}
	if c.IsSet("mongo-password") {
		MongoPassword = c.String("mongo-password")
	}
	if c.IsSet("nats-url") {
		NatsURL = c.String("nats-url")
	}
	if c.IsSet("platform-addr") {
		PlatformAddr = c.String("platform-addr")
	}
	if c.IsSet("platform-grpc-addr") {
		PlatformGrpcAddr = c.String("platform-grpc-addr")
	}
	if c.IsSet("platform-webhook-addr") {
		PlatformWebhookAddr = c.String("platform-webhook-addr")
	}
	if c.IsSet("gate-id") {
		GateID = c.Int("gate-id")
	}
	if c.IsSet("server-id") {
		ServerID = c.Int("server-id")
	}
	if c.IsSet("node-id") {
		NodeId = c.Int("node-id")
	}
	if c.IsSet("log_path") {
		LogPath = c.String("log_path")
	}

	return nil
}

func splitEndpoints(value string) []string {
	endpoints := strings.Split(value, ",")
	for i := range endpoints {
		endpoints[i] = strings.TrimSpace(endpoints[i])
	}
	return endpoints
}

func GateAddr() string {
	return fmt.Sprintf(":%d", GateBasePort+GateID)
}

func GameAddr() string {
	return fmt.Sprintf("%s:%d", GrpcHost, GameBasePort+ServerID)
}

func GrpcAddrFor(serverID int) string {
	return fmt.Sprintf("%s:%d", GrpcHost, GameBasePort+serverID)
}

package config

import (
	"fmt"
	"os"
	"strings"
)

var (
	// etcd 地址，支持环境变量 ETCD_ENDPOINTS 覆盖（逗号分隔）
	EtcdEndpoints = envSlice("ETCD_ENDPOINTS", "43.160.212.55:2379")

	// gate WebSocket 监听地址
	GateListenAddr = env("GATE_LISTEN_ADDR", ":8080")

	// game gRPC 基础端口，实际端口 = GrpcBasePort + ServerID
	GrpcBasePort = 9000

	// 日志级别
	LogLevel = env("LOG_LEVEL", "debug")

	// 区服 ID，由启动参数指定，每个 game 进程唯一
	ServerID = 0

	// game 进程对外暴露的 host（容器内需设为容器名或IP）
	GrpcHost = env("GRPC_HOST", "127.0.0.1")
)

func GrpcAddr() string {
	return fmt.Sprintf(":%d", GrpcBasePort+ServerID)
}

func GrpcAddrFor(serverID int) string {
	return fmt.Sprintf("%s:%d", GrpcHost, GrpcBasePort+serverID)
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

package main

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// streamDefs 定义所有业务 Stream，三个进程共享
var streamDefs = []nats.StreamConfig{
	{
		Name:     "GAME",
		Subjects: []string{"game.>"},  // 战斗结算、道具发放
		Storage:  nats.FileStorage,
		MaxAge:   24 * time.Hour,
	},
	{
		Name:     "MATCH",
		Subjects: []string{"match.>"},  // 匹配结果通知
		Storage:  nats.FileStorage,
		MaxAge:   1 * time.Hour,
	},
	{
		Name:     "CROSS",
		Subjects: []string{"cross.>"},  // 跨服消息
		Storage:  nats.FileStorage,
		MaxAge:   24 * time.Hour,
	},
}

// InitStreams 初始化所有 Stream，已存在则跳过，幂等安全
// 每个进程启动时调用一次即可
func InitStreams(js nats.JetStreamContext) {
	for _, cfg := range streamDefs {
		_, err := js.AddStream(&cfg)
		if err != nil && err != nats.ErrStreamNameAlreadyInUse {
			log.Fatalf("创建 Stream [%s] 失败: %v", cfg.Name, err)
		}
		log.Printf("Stream [%s] 就绪", cfg.Name)
	}
}

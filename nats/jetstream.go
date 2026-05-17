package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const natsURL = "nats://43.160.212.55:4222"

// BattleResult 战斗结果消息
type BattleResult struct {
	BattleID  string    `json:"battle_id"`
	WinnerID  string    `json:"winner_id"`
	LoserID   string    `json:"loser_id"`
	CreatedAt time.Time `json:"created_at"`
}

// publish 发布战斗结果到 JetStream
func publish(js nats.JetStreamContext, result BattleResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	ack, err := js.Publish("battle.result", data)
	if err != nil {
		return err
	}
	fmt.Printf("[发布] battle_id=%s seq=%d\n", result.BattleID, ack.Sequence)
	return nil
}

// consume 消费战斗结果，处理结算逻辑
func consume(js nats.JetStreamContext) {
	sub, err := js.Subscribe("battle.result", func(msg *nats.Msg) {
		var result BattleResult
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			log.Printf("解析消息失败: %v", err)
			msg.Nak() // 消费失败，重新投递
			return
		}
		fmt.Printf("[消费] battle_id=%s winner=%s loser=%s\n",
			result.BattleID, result.WinnerID, result.LoserID)

		// TODO: 在这里处理结算、道具发放等逻辑

		msg.Ack() // 确认消费成功
	}, nats.Durable("battle-consumer"), nats.ManualAck())
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	// 等待消息
	time.Sleep(3 * time.Second)
}

func jsTest() {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatal(err)
	}

	// 创建 Stream（只需创建一次，已存在会跳过）
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "BATTLE",
		Subjects: []string{"battle.>"},
		MaxAge:   24 * time.Hour,        // 消息保留 24 小时
		Storage:  nats.FileStorage,      // 文件存储，重启不丢数据
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		log.Fatal(err)
	}

	// 启动消费者
	go consume(js)

	// 模拟发布战斗结果
	publish(js, BattleResult{
		BattleID:  "battle-001",
		WinnerID:  "player-1",
		LoserID:   "player-2",
		CreatedAt: time.Now(),
	})

	time.Sleep(3 * time.Second)
}

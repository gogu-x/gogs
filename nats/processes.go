package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// ========== 游戏进程 ==========

func GameProcess(js nats.JetStreamContext) {
	// 订阅：接收匹配结果，开始战斗
	js.Subscribe("match.matched", func(msg *nats.Msg) {
		log.Printf("[游戏进程] 收到匹配结果: %s", msg.Data)
		msg.Ack()
	}, nats.Durable("game-recv-match"), nats.ManualAck())

	// 发布：战斗结束，通知结算
	result, _ := json.Marshal(map[string]string{
		"battle_id": "battle-001",
		"winner":    "player-1",
	})
	js.Publish("game.battle.result", result)
	log.Println("[游戏进程] 发布战斗结果")
}

// ========== 匹配进程 ==========

func MatchProcess(js nats.JetStreamContext) {
	// 订阅：接收战斗结果，更新匹配数据
	js.Subscribe("game.battle.result", func(msg *nats.Msg) {
		log.Printf("[匹配进程] 收到战斗结果: %s", msg.Data)
		msg.Ack()
	}, nats.Durable("match-recv-game"), nats.ManualAck())

	// 发布：匹配成功，通知游戏进程
	matched, _ := json.Marshal(map[string]string{
		"room_id":  "room-001",
		"player1":  "player-1",
		"player2":  "player-2",
	})
	js.Publish("match.matched", matched)
	log.Println("[匹配进程] 发布匹配结果")
}

// ========== 跨服进程 ==========

func CrossProcess(js nats.JetStreamContext) {
	// 订阅：同时监听游戏和匹配的消息
	js.Subscribe("game.>", func(msg *nats.Msg) {
		log.Printf("[跨服进程] 游戏消息 subject=%s data=%s", msg.Subject, msg.Data)
		msg.Ack()
	}, nats.Durable("cross-recv-game"), nats.ManualAck())

	js.Subscribe("match.>", func(msg *nats.Msg) {
		log.Printf("[跨服进程] 匹配消息 subject=%s data=%s", msg.Subject, msg.Data)
		msg.Ack()
	}, nats.Durable("cross-recv-match"), nats.ManualAck())

	// 发布：跨服事件
	js.Publish("cross.rank.update", []byte(`{"player":"player-1","score":100}`))
	log.Println("[跨服进程] 发布跨服排行榜更新")

	time.Sleep(2 * time.Second)
}

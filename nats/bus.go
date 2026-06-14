package natsclient

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// subject 规则统一定义
const (
	subjectGameIn       = "gate.in.%s"          // Gate → Game: gate.in.{serverID}
	subjectGateOut      = "gate.out.%s.%d"      // Game → Gate: gate.out.{gateID}.{connID}
	subjectCross        = "cross.%s"            // 跨服: cross.{topic}
	subjectGameShutdown = "game.shutdown.%s.%s" // Gate → Game: game.shutdown.{serverID}.{instID}
)

func subGameIn(serverID string) string { return fmt.Sprintf(subjectGameIn, serverID) }
func subGateOut(gateID string, connID uint64) string {
	return fmt.Sprintf(subjectGateOut, gateID, connID)
}
func subCross(topic string) string { return fmt.Sprintf(subjectCross, topic) }

// FrameHandler 处理从 NATS 收到的 proto.Message（已反序列化）
type FrameHandler func(data []byte, connID uint64, uid uint64)

// CrossHandler 处理跨服消息
type CrossHandler func(topic string, data []byte)

// PublishToGame Gate → Game，发送 proto.Message 序列化后的 bytes
func PublishToGame(serverID string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bus.PublishToGame marshal: %w", err)
	}
	subject := subGameIn(serverID)
	return nc.Publish(subject, data)
}

// SubscribeGameIn Game 侧订阅来自 Gate 的消息，同 serverID 多实例竞争消费
func SubscribeGameIn(serverID string, handler func(data []byte)) (*nats.Subscription, error) {
	return nc.Subscribe(subGameIn(serverID), func(msg *nats.Msg) {
		handler(msg.Data)
	})
}

// PublishToGate Game → Gate，发送 proto.Message 序列化后的 bytes
func PublishToGate(gateID string, connID uint64, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bus.PublishToGate marshal: %w", err)
	}
	return nc.Publish(subGateOut(gateID, connID), data)
}

// PublishRawToGate Game → Gate，直接发送已序列化的 bytes（避免重复序列化）
func PublishRawToGate(gateID string, connID uint64, data []byte) error {
	subject := subGateOut(gateID, connID)
	return nc.Publish(subject, data)
}

// SubscribeGateOut Gate 侧订阅本 Gate 的 Game 回包，workers 指定并发消费协程数
func SubscribeGateOut(gateID string, workers int, handler func(connID uint64, data []byte)) (*nats.Subscription, error) {
	subject := fmt.Sprintf("gate.out.%s.*", gateID)
	ch := make(chan *nats.Msg, 65536)
	sub, err := nc.ChanSubscribe(subject, ch)
	if err != nil {
		return nil, err
	}
	for i := 0; i < workers; i++ {
		go func() {
			for msg := range ch {
				parts := strings.Split(msg.Subject, ".")
				if len(parts) != 4 {
					continue
				}
				connID, err := strconv.ParseUint(parts[3], 10, 64)
				if err != nil {
					log.Printf("bus.SubscribeGateOut: invalid connID in subject %s", msg.Subject)
					continue
				}
				handler(connID, msg.Data)
			}
		}()
	}
	return sub, nil
}

// PublishCross 跨服广播，任意进程都可调用
func PublishCross(topic string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bus.PublishCross marshal: %w", err)
	}
	return nc.Publish(subCross(topic), data)
}

// SubscribeCross 订阅跨服消息，topic 支持通配符（如 ">" 订阅全部）
func SubscribeCross(topic string, handler func(topic string, data []byte)) (*nats.Subscription, error) {
	subject := subCross(topic)
	prefix := "cross."
	return nc.Subscribe(subject, func(msg *nats.Msg) {
		t := strings.TrimPrefix(msg.Subject, prefix)
		handler(t, msg.Data)
	})
}

// PublishShutdown Gate 通知指定 Game 实例退出
func PublishShutdown(serverID, instID string) error {
	return nc.Publish(fmt.Sprintf(subjectGameShutdown, serverID, instID), []byte("shutdown"))
}

// SubscribeShutdown Game 实例订阅自己的退出通知
func SubscribeShutdown(serverID, instID string, handler func()) (*nats.Subscription, error) {
	return nc.Subscribe(fmt.Sprintf(subjectGameShutdown, serverID, instID), func(_ *nats.Msg) {
		handler()
	})
}

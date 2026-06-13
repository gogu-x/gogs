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
	subjectGameIn  = "gate.in.%s"    // Gate → Game: gate.in.{serverID}
	subjectGateOut = "gate.out.%d"   // Game → Gate: gate.out.{connID}
	subjectCross   = "cross.%s"      // 跨服: cross.{topic}
)

func subGameIn(serverID string) string  { return fmt.Sprintf(subjectGameIn, serverID) }
func subGateOut(connID uint64) string   { return fmt.Sprintf(subjectGateOut, connID) }
func subCross(topic string) string      { return fmt.Sprintf(subjectCross, topic) }

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
	return nc.Publish(subGameIn(serverID), data)
}

// SubscribeGameIn Game 侧订阅来自 Gate 的消息，回调收到原始 bytes + 元数据
// unmarshal 由调用方指定，因为 Game 侧知道 codec
func SubscribeGameIn(serverID string, handler func(data []byte)) (*nats.Subscription, error) {
	return nc.Subscribe(subGameIn(serverID), func(msg *nats.Msg) {
		handler(msg.Data)
	})
}

// PublishToGate Game → Gate，发送 proto.Message 序列化后的 bytes
func PublishToGate(connID uint64, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bus.PublishToGate marshal: %w", err)
	}
	return nc.Publish(subGateOut(connID), data)
}

// PublishRawToGate Game → Gate，直接发送已序列化的 bytes（避免重复序列化）
func PublishRawToGate(connID uint64, data []byte) error {
	return nc.Publish(subGateOut(connID), data)
}

// SubscribeGateOut Gate 侧订阅所有 Game 回包，从 subject 解析 connID
func SubscribeGateOut(handler func(connID uint64, data []byte)) (*nats.Subscription, error) {
	return nc.Subscribe("gate.out.*", func(msg *nats.Msg) {
		parts := strings.Split(msg.Subject, ".")
		if len(parts) != 3 {
			return
		}
		connID, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {
			log.Printf("bus.SubscribeGateOut: invalid connID in subject %s", msg.Subject)
			return
		}
		handler(connID, msg.Data)
	})
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

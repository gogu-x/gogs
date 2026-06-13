package model

import (
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/app"
	natsclient "github.com/gogu-x/gogs/nats"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Consumer 全局单例，供 app.Context.Reply 调用
var Consumer *NatsConsumer

type NatsConsumer struct {
	sub *nats.Subscription
}

// Start 订阅 gate.in.{serverID}，将收到消息投递给 GameActor
func (c *NatsConsumer) Start(gamePID actor.PID) error {
	app.NatsReplier = c.Reply // 注入，避免 import cycle

	serverID := fmt.Sprintf("%d", config.ServerID)
	sub, err := natsclient.SubscribeGameIn(serverID, func(data []byte) {
		var frame protoGateway.Frame
		if err := proto.Unmarshal(data, &frame); err != nil {
			log.Printf("NatsConsumer: unmarshal frame error: %v", err)
			return
		}
		if len(frame.Payload) == 0 {
			return
		}
		inner, err := codec.ProtoCodec.Unmarshal(frame.Payload)
		if err != nil {
			log.Printf("NatsConsumer: unmarshal payload error: %v", err)
			return
		}
		protoMsg, ok := inner.(proto.Message)
		if !ok {
			log.Printf("NatsConsumer: payload is not proto.Message")
			return
		}
		actor.Send(gamePID, &inboundMsg{msg: protoMsg, uid: frame.Uid, connID: frame.ConnId})
	})
	if err != nil {
		return err
	}
	c.sub = sub
	log.Printf("NatsConsumer: subscribed gate.in.%s", serverID)
	return nil
}

// Reply 通过 bus 回包给 Gate，payload 是已序列化的 protoGateway.Frame
func (c *NatsConsumer) Reply(connID uint64, payload []byte) error {
	return natsclient.PublishRawToGate(connID, payload)
}

func (c *NatsConsumer) Stop() {
	if c.sub != nil {
		c.sub.Unsubscribe()
	}
}

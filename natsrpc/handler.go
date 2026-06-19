package natsrpc

import (
	"log"
	"strconv"
	"strings"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

// ─── Router handler 方法 ─────────────────────────────────────────────────────

// handleInbound Game 侧：解码后的消息分发给业务 Actor
func (a *Actor) handleInbound(ctx actor.ActorContext, msg interface{}) {
	m := msg.(*InboundMsg)
	dispatchProto(ctx, m.Msg, Meta{UID: m.UID, ConnID: m.ConnID, GateID: m.GateID})
}

// handleOutbound Gate 侧：ConnActor 发来的消息，转发到 Game
func (a *Actor) handleOutbound(_ actor.ActorContext, msg interface{}) {
	m := msg.(*OutboundMsg)
	if err := PublishToGame(m.Frame.ServerId, m.Frame); err != nil {
		log.Printf("natsrpc: publish to game: %v", err)
	}
}

// handleRaw Gate 侧：NATS 收到的原始帧，找到对应 ConnActor 投递
func (a *Actor) handleRaw(_ actor.ActorContext, msg interface{}) {
	m := msg.(*rawMsg)
	if a.cfg.LookupConn == nil {
		return
	}
	pid, ok := a.cfg.LookupConn(m.connID)
	if !ok {
		return
	}
	var frame protoGateway.Frame
	if err := proto.Unmarshal(m.data, &frame); err == nil {
		actor.Send(pid, &frame)
	}
}

// handleReply Game 侧：业务 Actor 回包，发布到 Gate 的 NATS 主题
func (a *Actor) handleReply(_ actor.ActorContext, msg interface{}) {
	m := msg.(*ReplyMsg)
	var frame protoGateway.Frame
	if err := proto.Unmarshal(m.Data, &frame); err != nil {
		log.Printf("natsrpc: reply unmarshal: %v", err)
		return
	}
	if err := PublishRawToGate(frame.GateId, frame.ConnId, m.Data); err != nil {
		log.Printf("natsrpc: reply publish: %v", err)
	}
}

func (a *Actor) handleShutdown(ctx actor.ActorContext, _ interface{}) {
	log.Printf("natsrpc: shutdown signal received")
	a.OnStop(ctx)
}

// ─── NATS 订阅回调 ────────────────────────────────────────────────────────────

// onGameInMsg Game 侧订阅 gate.in.{serverID}，解码后转为 InboundMsg 投递给自己
func (a *Actor) onGameInMsg(self actor.PID, m *natsgo.Msg) {
	var frame protoGateway.Frame
	if err := proto.Unmarshal(m.Data, &frame); err != nil {
		log.Printf("natsrpc: unmarshal frame: %v", err)
		return
	}
	if len(frame.Payload) == 0 {
		return
	}
	inner, err := codec.ProtoCodec.Unmarshal(frame.Payload)
	if err != nil {
		log.Printf("natsrpc: unmarshal payload: %v", err)
		return
	}
	protoMsg, ok := inner.(proto.Message)
	if !ok {
		return
	}
	actor.Send(self, &InboundMsg{Msg: protoMsg, UID: frame.Uid, ConnID: frame.ConnId, GateID: frame.GateId})
}

// runGateOutWorker Gate 侧消费 gate.out.{gateID}.* channel，解出 connID 后投递 rawMsg
func (a *Actor) runGateOutWorker(self actor.PID, ch <-chan *natsgo.Msg) {
	for m := range ch {
		parts := strings.Split(m.Subject, ".")
		if len(parts) != 4 {
			continue
		}
		connID, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil {
			continue
		}
		actor.Send(self, &rawMsg{connID: connID, data: m.Data})
	}
}

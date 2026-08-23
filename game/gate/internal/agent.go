package internal

import (
	"fmt"
	"log"
	"time"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

const maxFramesPerSecond = 200

type sessionKey struct {
	gateID string
	connID uint64
}

func (k sessionKey) String() string {
	return fmt.Sprintf("%s/%d", k.gateID, k.connID)
}

type session struct {
	pid        tree.PID
	generation uint64
	uid        uint64
}

type openSession struct {
	stream protoGateway.Gateway_StreamServer
	first  *protoGateway.Frame
}

type openedSession struct {
	key        sessionKey
	pid        tree.PID
	generation uint64
}

type closeSession struct {
	key        sessionKey
	pid        tree.PID
	generation uint64
	reason     string
}

type sessionUID struct {
	key        sessionKey
	pid        tree.PID
	generation uint64
	uid        uint64
}

type inboundFrame struct {
	frame *protoGateway.Frame
}

type outboundFrame struct {
	frame *protoGateway.Frame
}

type stopAgent struct{}

// PushToConn is the Game-side message used to send a frame to a live gateway
// connection. GateActor resolves the session key and routes it to its agent.
type PushToConn struct {
	GateID string
	ConnID uint64
	Frame  *protoGateway.Frame
}

// BanUID and UnbanUID are control messages for the in-memory session ban list.
// Persistent ban decisions should send one of these messages to GateActor.
type BanUID struct{ UID uint64 }
type UnbanUID struct{ UID uint64 }

// GsAgent owns one gRPC stream. Its mailbox serializes inbound client frames
// and outbound Game pushes so stream.Send is never called concurrently.
type GsAgent struct {
	key        sessionKey
	generation uint64
	stream     protoGateway.Gateway_StreamServer
	manager    tree.PID
	router     tree.Router

	uid         uint64
	windowStart time.Time
	frameCount  int
}

func newGsAgent(key sessionKey, generation uint64, stream protoGateway.Gateway_StreamServer, manager tree.PID) *GsAgent {
	return &GsAgent{
		key:         key,
		generation:  generation,
		stream:      stream,
		manager:     manager,
		windowStart: time.Now(),
	}
}

func (g *GsAgent) Name() string {
	return fmt.Sprintf("gateway-conn-%s-%d-%d", g.key.gateID, g.key.connID, g.generation)
}

func (g *GsAgent) MailboxSize() int { return 256 }

func (g *GsAgent) OnInit(_ tree.Context) {
	g.router.Register(&inboundFrame{}, g.handleInboundFrame)
	g.router.Register(&outboundFrame{}, g.handleOutboundFrame)
	g.router.Register(&stopAgent{}, g.handleStop)
}

func (g *GsAgent) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GsAgent) OnStop(_ tree.Context) {
	g.stream = nil
}

func (g *GsAgent) handleInboundFrame(ctx tree.Context, msg interface{}) {
	g.onInbound(ctx, msg.(*inboundFrame).frame)
}

func (g *GsAgent) handleOutboundFrame(ctx tree.Context, msg interface{}) {
	g.onOutbound(ctx, msg.(*outboundFrame).frame)
}

func (g *GsAgent) handleStop(ctx tree.Context, _ interface{}) {
	ctx.Stop()
}

func (g *GsAgent) onInbound(ctx tree.Context, frame *protoGateway.Frame) {
	if frame == nil || frame.GetConnId() != g.key.connID || frame.GetGateId() != g.key.gateID || frame.GetServerId() != int32(config.ServerID) {
		g.close(ctx, "invalid connection ownership")
		return
	}
	if !g.allowFrame() {
		g.close(ctx, "inbound rate limit exceeded")
		return
	}
	if frame.GetUid() != 0 && frame.GetUid() != g.uid {
		binding := ctx.Request(g.manager, &sessionUID{
			key:        g.key,
			pid:        ctx.Self(),
			generation: g.generation,
			uid:        frame.GetUid(),
		})
		result, err := binding.AwaitTimeout(2 * time.Second)
		allowed, ok := result.(bool)
		if err != nil || !ok || !allowed {
			g.close(ctx, "session UID rejected")
			return
		}
		g.uid = frame.GetUid()
	}

	playPID, ok := ctx.Lookup(constant.PLAY)
	if !ok {
		g.close(ctx, "Play actor is unavailable")
		return
	}
	if !ctx.Send(playPID, frame) {
		g.close(ctx, "Play actor stopped")
	}
}

func (g *GsAgent) onOutbound(ctx tree.Context, frame *protoGateway.Frame) {
	if frame == nil || g.stream == nil {
		return
	}

	out, ok := proto.Clone(frame).(*protoGateway.Frame)
	if !ok {
		log.Printf("GatewayAgent[%s]: clone outbound frame failed", g.key)
		return
	}
	out.GateId = g.key.gateID
	out.ConnId = g.key.connID
	out.ServerId = int32(config.ServerID)
	if g.uid != 0 {
		out.Uid = g.uid
	}
	if err := g.stream.Send(out); err != nil {
		g.close(ctx, fmt.Sprintf("stream send: %v", err))
	}
}

func (g *GsAgent) allowFrame() bool {
	now := time.Now()
	if now.Sub(g.windowStart) >= time.Second {
		g.windowStart = now
		g.frameCount = 0
	}
	g.frameCount++
	return g.frameCount <= maxFramesPerSecond
}

func (g *GsAgent) close(ctx tree.Context, reason string) {
	ctx.Send(g.manager, &closeSession{
		key:        g.key,
		pid:        ctx.Self(),
		generation: g.generation,
		reason:     reason,
	})
	ctx.Stop()
}

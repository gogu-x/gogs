package internal

import (
	"fmt"
	"log"
	"time"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"google.golang.org/protobuf/proto"
)

const maxFramesPerSecond = 200

type session struct {
	pid        tree.PID
	generation uint64
	uid        uint64
}

type openSession struct {
	stream pb_gateway.Gateway_StreamServer
	frame  *pb_gateway.Frame
}

type openedSession struct {
	pid        tree.PID
	uid        uint64
	generation uint64
}

type closeSession struct {
	pid        tree.PID
	uid        uint64
	generation uint64
	reason     string
}

type inboundFrame struct {
	frame *pb_gateway.Frame
}

type stopAgent struct{}

// PushToMsg routes a Game-side frame to the live stream identified by UID.
type PushToMsg struct {
	UID   uint64
	Frame *pb_gateway.Frame
}

// BanUID and UnbanUID are control messages for the in-memory session ban list.
type BanUID struct{ UID uint64 }
type UnbanUID struct{ UID uint64 }

// GsAgent owns the gRPC stream for one authenticated UID.
type GsAgent struct {
	generation  uint64
	stream      pb_gateway.Gateway_StreamServer
	manager     tree.PID
	router      tree.Router
	uid         uint64
	windowStart time.Time
	frameCount  int
}

func newGsAgent(
	uid uint64,
	generation uint64,
	stream pb_gateway.Gateway_StreamServer,
	manager tree.PID,
) *GsAgent {
	return &GsAgent{
		uid:         uid,
		generation:  generation,
		stream:      stream,
		manager:     manager,
		windowStart: time.Now(),
	}
}

func (g *GsAgent) Name() string {
	return fmt.Sprintf("gateway-uid-%d-%d", g.uid, g.generation)
}

func (g *GsAgent) MailboxSize() int { return 256 }

func (g *GsAgent) OnInit(_ tree.Context) {
	g.router.Register(&inboundFrame{}, g.handleInboundFrame)
	g.router.Register(&pb_gateway.Frame{}, g.handleOutboundFrame)
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
	g.onOutbound(ctx, msg.(*pb_gateway.Frame))
}

func (g *GsAgent) handleStop(ctx tree.Context, _ interface{}) {
	ctx.Stop()
}

func (g *GsAgent) onInbound(ctx tree.Context, frame *pb_gateway.Frame) {
	if !g.allowFrame() {
		g.close(ctx, "inbound rate limit exceeded")
		return
	}

	msg, uid, err := decodeInboundFrame(frame)
	if err != nil {
		g.close(ctx, fmt.Sprintf("invalid inbound frame: %v", err))
		return
	}
	if uid != g.uid {
		g.close(ctx, fmt.Sprintf("message UID mismatch: got %d want %d", uid, g.uid))
		return
	}

	playPID, ok := ctx.Lookup(constant.PLAY)
	if !ok {
		g.close(ctx, "Play actor is unavailable")
		return
	}
	if !ctx.Send(playPID, msg) {
		g.close(ctx, "Play actor stopped")
	}
}

func (g *GsAgent) onOutbound(ctx tree.Context, frame *pb_gateway.Frame) {
	if frame == nil || g.stream == nil {
		return
	}

	out, ok := proto.Clone(frame).(*pb_gateway.Frame)
	if !ok {
		log.Printf("GatewayAgent[%d]: clone outbound frame failed", g.uid)
		return
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
		uid:        g.uid,
		pid:        ctx.Self(),
		generation: g.generation,
		reason:     reason,
	})
	ctx.Stop()
}

func decodeInboundFrame(frame *pb_gateway.Frame) (interface{}, uint64, error) {
	if err := validateFrame(frame); err != nil {
		return nil, 0, err
	}
	msg, err := codec.ProtoCodec.Unmarshal(frame.GetPayload())
	if err != nil {
		return nil, 0, fmt.Errorf("decode payload: %w", err)
	}
	request, ok := msg.(interface{ GetUID() uint64 })
	if !ok {
		return nil, 0, fmt.Errorf("message %T has no UID", msg)
	}
	uid := request.GetUID()
	if uid == 0 {
		return nil, 0, fmt.Errorf("message %T has an empty UID", msg)
	}
	return msg, uid, nil
}

package internal

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/tlog"
)

const maxFramesPerSecond = 200

type session struct {
	pid         tree.PID
	playerPID   tree.PID
	activityPID tree.PID
	generation  uint64
	uid         uint64
}

type openSession struct {
	stream  pb_gateway.Gateway_StreamServer
	msg     interface{}
	request *tree.Envelope
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
	msg interface{}
}

type stopAgent struct{}

// GsAgent owns the gRPC stream for one authenticated UID.
type GsAgent struct {
	generation  uint64
	stream      pb_gateway.Gateway_StreamServer
	manager     tree.PID
	playerPID   tree.PID
	activityPID tree.PID
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
	playerPID tree.PID,
	activityPID tree.PID,
) *GsAgent {
	return &GsAgent{
		uid:         uid,
		generation:  generation,
		stream:      stream,
		manager:     manager,
		playerPID:   playerPID,
		activityPID: activityPID,
		windowStart: time.Now(),
	}
}

func (g *GsAgent) Name() string {
	return fmt.Sprintf("gateway-uid-%d-%d", g.uid, g.generation)
}

// MailboxSize A battle report is published as a short burst of individual events. Keep
// enough ordered outbound frames to let the gRPC writer drain that burst.
func (g *GsAgent) MailboxSize() int { return 2 * 1024 }

func (g *GsAgent) OnInit(_ tree.Context) {
	g.router.Register(&inboundFrame{}, g.handleInboundFrame)
	g.router.Register(&ipb.PushToMsg{}, g.handleOutboundFrame)
	g.router.Register(&stopAgent{}, g.handleStop)
}

func (g *GsAgent) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GsAgent) OnStop(_ tree.Context) {
	g.stream = nil
}

func (g *GsAgent) handleInboundFrame(ctx tree.Context, msg interface{}) {
	g.onInbound(ctx, msg)
}

func (g *GsAgent) handleOutboundFrame(ctx tree.Context, msg interface{}) {
	g.onOutbound(ctx, msg)
}

func (g *GsAgent) handleStop(ctx tree.Context, _ interface{}) {
	ctx.Stop()
}

func (g *GsAgent) onInbound(ctx tree.Context, msg interface{}) {
	if !g.allowFrame() {
		g.close(ctx, "inbound rate limit exceeded")
		return
	}

	msgInfo, ok := msg.(*inboundFrame)
	if !ok || msgInfo == nil || msgInfo.msg == nil {
		g.close(ctx, "inbound msg exceeded")
		return
	}

	targetPID := g.playerPID
	switch msgInfo.msg.(type) {
	case *pb_activity.GetActivityListReq:
		targetPID = g.activityPID
	case *pb_activity.JoinActivityReq:
		targetPID = g.activityPID
	case *pb_activity.GetProgressReq:
		targetPID = g.activityPID
	case *pb_activity.ClaimRewardReq:
		targetPID = g.activityPID
	}
	if targetPID.ID == 0 {
		tlog.Log.Error("[gate/GsAgent.onInbound] 玩家业务 Actor PID 缺失, pid=%v type=%T", g.uid, msgInfo.msg)
		g.close(ctx, "player actor is unavailable")
		return
	}
	inbound := &ipb.PlayerInbound{
		UID:        g.uid,
		Generation: g.generation,
		Message:    msgInfo.msg,
	}
	tlog.Log.Debug("[gate/GsAgent.onInbound] 收到玩家请求, pid=%v generation=%v type=%T", g.uid, g.generation, msgInfo.msg)
	if !ctx.TrySend(targetPID, inbound) {
		tlog.Log.Error("[gate/GsAgent.onInbound] 玩家业务 Actor 不可用或队列已满, pid=%v actorPID=%v type=%T", g.uid, targetPID, msgInfo.msg)
		g.close(ctx, "player actor is unavailable or overloaded")
	}
}

// 发送到gate消息
func (g *GsAgent) onOutbound(ctx tree.Context, msg interface{}) {
	if g.stream == nil {
		return
	}
	//解码然后发送
	msgInfo, ok := msg.(*ipb.PushToMsg)
	if !ok {
		return
	}
	data, err := codec.ProtoCodec.Marshal(msgInfo.Msg)
	if err != nil {
		return
	}
	frame := &pb_gateway.Frame{Payload: data}
	if err := g.stream.Send(frame); err != nil {
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

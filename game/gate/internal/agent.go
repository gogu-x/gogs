package internal

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/tlog"
)

// maxFramesPerSecond 表示单个网关会话每秒允许接收的最大帧数。
const maxFramesPerSecond = 200

type session struct {
	pid        tree.PID
	playerPID  tree.PID
	generation uint64
	uid        uint64
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

// GsAgent 管理一个已认证玩家的 gRPC 会话。
type GsAgent struct {
	generation  uint64
	stream      pb_gateway.Gateway_StreamServer
	manager     tree.PID
	playerPID   tree.PID
	router      tree.Router
	uid         uint64
	windowStart time.Time
	frameCount  int
}

// newGsAgent 创建绑定玩家 Actor 的网关会话代理。
func newGsAgent(
	uid uint64,
	generation uint64,
	stream pb_gateway.Gateway_StreamServer,
	manager tree.PID,
	playerPID tree.PID,
) *GsAgent {
	return &GsAgent{
		uid:         uid,
		generation:  generation,
		stream:      stream,
		manager:     manager,
		playerPID:   playerPID,
		windowStart: time.Now(),
	}
}

// Name 返回当前网关会话代理的唯一名称。
func (g *GsAgent) Name() string {
	return fmt.Sprintf("gateway-uid-%d-%d", g.uid, g.generation)
}

// MailboxSize 为批量战报等连续推送保留足够的有序出站消息队列。
func (g *GsAgent) MailboxSize() int { return 2 * 1024 }

// OnInit 注册入站、出站和停止消息的处理方法。
func (g *GsAgent) OnInit(_ tree.Context) {
	g.router.Register(&inboundFrame{}, g.handleInboundFrame)
	g.router.Register(&ipb.PushToMsg{}, g.handleOutboundFrame)
	g.router.Register(&stopAgent{}, g.handleStop)
}

// HandleMessage 将网关会话消息分发给已注册的处理方法。
func (g *GsAgent) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

// OnStop 释放网关会话持有的流引用。
func (g *GsAgent) OnStop(_ tree.Context) {
	g.stream = nil
}

// handleInboundFrame 处理从客户端读取的入站帧。
func (g *GsAgent) handleInboundFrame(ctx tree.Context, msg interface{}) {
	g.onInbound(ctx, msg)
}

// handleOutboundFrame 处理需要写入客户端的出站消息。
func (g *GsAgent) handleOutboundFrame(ctx tree.Context, msg interface{}) {
	g.onOutbound(ctx, msg)
}

// handleStop 停止当前网关会话代理。
func (g *GsAgent) handleStop(ctx tree.Context, _ interface{}) {
	ctx.Stop()
}

// onInbound 校验客户端帧并将请求发送到玩家 Actor。
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
	// 解码后发送客户端协议消息。
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

// allowFrame 检查当前会话是否超过每秒入站帧数限制。
func (g *GsAgent) allowFrame() bool {
	now := time.Now()
	if now.Sub(g.windowStart) >= time.Second {
		g.windowStart = now
		g.frameCount = 0
	}
	g.frameCount++
	return g.frameCount <= maxFramesPerSecond
}

// close 通知网关关闭当前会话并记录关闭原因。
func (g *GsAgent) close(ctx tree.Context, reason string) {
	ctx.Send(g.manager, &closeSession{
		uid:        g.uid,
		pid:        ctx.Self(),
		generation: g.generation,
		reason:     reason,
	})
	ctx.Stop()
}

package natsrpc

import (
	"fmt"
	"log"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	actor "github.com/gogu-x/bigTree"
)

// RouteFunc 根据 Frame 返回目标 Actor PID。
type RouteFunc func(frame *Frame) (actor.PID, bool)

// ActorConfig 配置订阅列表。
type ActorConfig struct {
	Subs []SubConfig
}

// pending 记录一条等待回包的跨节点请求
type pending struct {
	callerPID actor.PID
	cb        func(interface{}, error)
}

// replyFrame subscribe worker 收到带 RequestId 的回包，投给 NatsActor 处理
type replyFrame struct{ frame *Frame }

// timeoutMsg 超时信号，投给 NatsActor 处理
type timeoutMsg struct{ requestId string }

// Actor NATS 订阅 Actor，负责收消息、反序列化、投递。
type Actor struct {
	cfg        ActorConfig
	subs       []*natsgo.Subscription
	pendingMap map[string]*pending // 只在 NatsActor goroutine 访问，无需锁
}

func NewActor(cfg ActorConfig) *Actor {
	return &Actor{cfg: cfg, pendingMap: make(map[string]*pending)}
}

func (a *Actor) OnInit(ctx actor.ActorContext) {
	self := ctx.Self()
	for _, sub := range a.cfg.Subs {
		switch sub.kind {
		case kindSub:
			a.subscribe(self, sub)
		case kindShutdown:
			a.subscribeShutdown(self, sub)
		}
	}
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *SendMsg:
		if err := send(m); err != nil {
			log.Printf("natsrpc: send [%s/%s]: %v", m.Module, m.ID, err)
		}
	case *RequestMsg:
		a.handleRequest(ctx, m)
	case *replyFrame:
		a.handleReply(m.frame)
	case *timeoutMsg:
		a.handleTimeout(m.requestId)
	case *shutdownMsg:
		a.OnStop(ctx)
	}
}

func (a *Actor) OnStop(_ actor.ActorContext) {
	for _, sub := range a.subs {
		_ = sub.Unsubscribe()
	}
	actor.Default().Shutdown()
}

func (a *Actor) subscribe(self actor.PID, sub SubConfig) {
	ch := make(chan *natsgo.Msg, 65536)
	s, err := nc.ChanSubscribe(sub.subject, ch)
	if err != nil {
		log.Fatalf("natsrpc: subscribe %s: %v", sub.subject, err)
	}
	a.subs = append(a.subs, s)
	workers := sub.workers
	if workers <= 0 {
		workers = 1
	}
	route := sub.route
	for i := 0; i < workers; i++ {
		go func() {
			for m := range ch {
				var frame Frame
				if err := proto.Unmarshal(m.Data, &frame); err != nil {
					log.Printf("natsrpc: unmarshal frame: %v", err)
					continue
				}
				// 有 RequestId → 回包，投给 NatsActor 串行处理
				if frame.RequestId != "" {
					actor.Send(self, &replyFrame{frame: &frame})
					continue
				}
				pid, ok := route(&frame)
				if !ok {
					continue
				}
				actor.Default().Send(pid, &frame)
			}
		}()
	}
	log.Printf("natsrpc: subscribed %s (%d workers)", sub.subject, workers)
}

// handleRequest 在 NatsActor goroutine 内执行：生成 requestId，存 pending，发消息
func (a *Actor) handleRequest(ctx actor.ActorContext, m *RequestMsg) {
	requestId := newRequestId()
	m.Frame.RequestId = requestId

	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	a.pendingMap[requestId] = &pending{callerPID: m.CallerPID, cb: m.Callback}

	// 超时时投 timeoutMsg 给自己，串行处理
	self := ctx.Self()
	ctx.AfterFunc(timeout, func(_ actor.ActorContext) {
		actor.Send(self, &timeoutMsg{requestId: requestId})
	})

	if err := send(&SendMsg{Module: m.Module, ID: m.ID, NodeId: m.NodeId, Frame: m.Frame}); err != nil {
		delete(a.pendingMap, requestId)
		actor.SendCallback(m.CallerPID, m.Callback, nil, err)
	}
}

// handleReply 在 NatsActor goroutine 内执行：查 pending，触发回调
func (a *Actor) handleReply(frame *Frame) {
	p, ok := a.pendingMap[frame.RequestId]
	if !ok {
		return
	}
	delete(a.pendingMap, frame.RequestId)
	actor.SendCallback(p.callerPID, p.cb, frame.Payload, nil)
}

// handleTimeout 在 NatsActor goroutine 内执行：超时回调
func (a *Actor) handleTimeout(requestId string) {
	p, ok := a.pendingMap[requestId]
	if !ok {
		return // 已被 handleReply 处理，忽略
	}
	delete(a.pendingMap, requestId)
	actor.SendCallback(p.callerPID, p.cb, nil, fmt.Errorf("natsrpc: request timeout [%s]", requestId))
}

func (a *Actor) subscribeShutdown(self actor.PID, sub SubConfig) {
	s, err := nc.Subscribe(sub.subject, func(_ *natsgo.Msg) {
		actor.Send(self, &shutdownMsg{})
	})
	if err != nil {
		log.Printf("natsrpc: subscribe %s: %v", sub.subject, err)
		return
	}
	a.subs = append(a.subs, s)
	log.Printf("natsrpc: subscribed %s", sub.subject)
}

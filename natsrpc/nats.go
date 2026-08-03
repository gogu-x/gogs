package natsrpc

import (
	"fmt"
	"log"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/tree"
)

// RouteFunc 根据 Frame 返回目标 Actor PID。
type RouteFunc func(frame *Frame) (tree.PID, bool)

// ActorConfig 配置订阅列表。
type ActorConfig struct {
	// Name 注册名，为空时使用 constant.ActorNats（每个进程一个 NatsActor）。
	Name string
	Subs []SubConfig
}

// pending 记录一条等待回包的跨节点请求（Call 模式）。
type pending struct {
	callerPID tree.PID
	cb        func(*Frame, error)
}

// replyFrame subscribe worker 收到私有 inbox 的回包，投给 NatsActor 处理。
// subject 是回包实际投递到的 NATS subject（用于匹配 pendingMap），
// 不依赖回包 Frame 自身是否携带 RequestId 字段。
type replyFrame struct {
	subject string
	frame   *Frame
}

// timeoutMsg 超时信号，投给 NatsActor 处理。
type timeoutMsg struct{ requestId string }

// Actor NATS 订阅 Actor，负责收消息、反序列化、投递，并承载 Cast/Call 的发送与超时管理。
type Actor struct {
	cfg        ActorConfig
	subs       []*natsgo.Subscription
	pendingMap map[string]*pending // 只在 NatsActor goroutine 访问，无需锁
	inboxBase  string              // 本 NatsActor 私有回包 inbox 前缀，如 _INBOX.xxxxx
	inboxSub   *natsgo.Subscription
	inboxCh    chan *natsgo.Msg
}

func NewActor(cfg ActorConfig) *Actor {
	return &Actor{cfg: cfg, pendingMap: make(map[string]*pending)}
}

// Name 实现 tree.Actor，注册名默认为 constant.ActorNats。
func (a *Actor) Name() string {
	if a.cfg.Name != "" {
		return a.cfg.Name
	}
	return constant.Nats
}

func (a *Actor) OnInit(ctx tree.Context) {
	self := ctx.Self()
	for _, sub := range a.cfg.Subs {
		switch sub.kind {
		case kindSub:
			a.subscribe(self, sub)
		case kindShutdown:
			a.subscribeShutdown(self, sub)
		}
	}
	// 本模块的 Call 回包统一走一个私有 inbox：{inbox}.>，每次 Call 生成唯一子 subject。
	a.subscribeInbox(self)
}

func (a *Actor) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *castMsg:
		if err := publishTo(m.Module, m.ID, m.NodeId, m.Frame); err != nil {
			log.Printf("natsrpc: cast [%s/%s/%s]: %v", m.Module, m.ID, m.NodeId, err)
		}
	case *callMsg:
		a.handleCall(ctx, m)
	case *replyFrame:
		a.handleReply(m.subject, m.frame)
	case *timeoutMsg:
		a.handleTimeout(m.requestId)
	case *shutdownMsg:
		a.OnStop(ctx)
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	for _, sub := range a.subs {
		_ = sub.Unsubscribe()
	}
	if a.inboxSub != nil {
		_ = a.inboxSub.Unsubscribe()
	}
	tree.Default().Shutdown()
}

func (a *Actor) subscribe(self tree.PID, sub SubConfig) {
	ch := make(chan *natsgo.Msg, 128)
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
				// CallSync 走 NATS 原生 request-reply，回复目标在 m.Reply；
				// Call 模式则已经把私有 inbox subject 编码进 frame.RequestId，不可覆盖。
				if frame.RequestId == "" && m.Reply != "" {
					frame.RequestId = m.Reply
				}
				pid, ok := route(&frame)
				if !ok {
					continue
				}
				tree.Default().Send(pid, &frame)
			}
		}()
	}
	log.Printf("natsrpc: subscribed %s (%d workers)", sub.subject, workers)
}

// subscribeInbox 订阅本 NatsActor 私有回包 inbox（Call 模式的回复统一进这里）。
func (a *Actor) subscribeInbox(self tree.PID) {
	inbox := natsgo.NewInbox() // _INBOX.<uid>
	ch := make(chan *natsgo.Msg, 128)
	s, err := nc.ChanSubscribe(inbox+".*", ch)
	if err != nil {
		log.Fatalf("natsrpc: subscribe inbox %s: %v", inbox, err)
	}
	a.inboxBase = inbox
	a.inboxSub = s
	a.inboxCh = ch
	go func() {
		for m := range ch {
			var frame Frame
			if err := proto.Unmarshal(m.Data, &frame); err != nil {
				log.Printf("natsrpc: unmarshal reply frame: %v", err)
				continue
			}
			tree.Send(self, &replyFrame{subject: m.Subject, frame: &frame})
		}
	}()
}

// handleCall 在 NatsActor goroutine 内执行：生成 reply inbox subject，存 pending，发消息。
func (a *Actor) handleCall(ctx tree.Context, m *callMsg) {
	requestId := a.inboxBase + "." + newRequestId()
	m.Frame.RequestId = requestId

	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	a.pendingMap[requestId] = &pending{callerPID: m.CallerPID, cb: m.Callback}

	// 超时时投 timeoutMsg 给自己，串行处理。
	self := ctx.Self()
	ctx.AfterFunc(timeout, func(_ tree.Context) {
		tree.Send(self, &timeoutMsg{requestId: requestId})
	})

	if err := publishTo(m.Module, m.ID, m.NodeId, m.Frame); err != nil {
		delete(a.pendingMap, requestId)
		sendFrameCallback(m.CallerPID, m.Callback, nil, err)
	}
}

// handleReply 在 NatsActor goroutine 内执行：按接收 subject 查 pending，触发回调。
func (a *Actor) handleReply(subject string, frame *Frame) {
	p, ok := a.pendingMap[subject]
	if !ok {
		return
	}
	delete(a.pendingMap, subject)
	sendFrameCallback(p.callerPID, p.cb, frame, nil)
}

// handleTimeout 在 NatsActor goroutine 内执行：超时回调。
func (a *Actor) handleTimeout(requestId string) {
	p, ok := a.pendingMap[requestId]
	if !ok {
		return // 已被 handleReply 处理，忽略
	}
	delete(a.pendingMap, requestId)
	sendFrameCallback(p.callerPID, p.cb, nil, fmt.Errorf("natsrpc: request timeout [%s]", requestId))
}

func (a *Actor) subscribeShutdown(self tree.PID, sub SubConfig) {
	s, err := nc.Subscribe(sub.subject, func(_ *natsgo.Msg) {
		tree.Send(self, &shutdownMsg{})
	})
	if err != nil {
		log.Printf("natsrpc: subscribe %s: %v", sub.subject, err)
		return
	}
	a.subs = append(a.subs, s)
	log.Printf("natsrpc: subscribed %s", sub.subject)
}

// sendFrameCallback 包装 actor.SendCallback，适配 func(*Frame, error) 签名。
func sendFrameCallback(pid tree.PID, cb func(*Frame, error), frame *Frame, err error) bool {
	return false
}

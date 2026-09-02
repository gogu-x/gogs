package natsrpc

import (
	"log"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/timer"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const natsRequestTimeout = timer.TimerType(1)

// ActorConfig 配置订阅列表。
type ActorConfig struct {
	// Name 注册名，为空时使用 constant.ActorNats（每个进程一个 NatsActor）。
	Name string
	Subs []SubConfig
}

// pending 记录一条等待回包的跨节点请求（Call 模式）。
type pending struct {
	callerPID tree.PID
	cb        func(proto.Message, error)
}

// Actor NATS 订阅 Actor，负责收消息、反序列化、投递，并承载 Cast/Call 的发送与超时管理。
type Actor struct {
	cfg        ActorConfig
	subs       []*natsgo.Subscription
	pendingMap map[string]*pending // 只在 NatsActor goroutine 访问，无需锁
	inboxBase  string              // 本 NatsActor 私有回包 inbox 前缀，如 _INBOX.xxxxx
	inboxSub   *natsgo.Subscription
	inboxCh    chan *natsgo.Msg
	timeWheel  *timer.TimeWheel
	codec      codec.Codec
}

func NewActor(cfg ActorConfig) *Actor {
	return &Actor{cfg: cfg, pendingMap: make(map[string]*pending), codec: codec.ProtoCodec}
}

// Name 实现 tree.Actor，注册名默认为 constant.ActorNats。
func (a *Actor) Name() string {
	if a.cfg.Name != "" {
		return a.cfg.Name
	}
	return def.Nats
}

func (a *Actor) OnInit(ctx tree.Context) {
	a.timeWheel = timer.NewTimeWheel(1024, ctx.Self(), ctx.System())
	a.timeWheel.Register(natsRequestTimeout, func(data interface{}) {
		//requestID, ok := data.(string)
		//if !ok {
		//	def.DLog.Info("natsrpc: invalid timeout timer data %T", data)
		//	return
		//}
		//a.handleTimeout(requestID)
	})
	self := ctx.Self()
	//根据订阅创建nats消息监听
	for _, sub := range a.cfg.Subs {
		switch sub.kind {
		case kindSub:
			a.subscribe(self, sub)
			//case kindShutdown:
			//	a.subscribeShutdown(self, sub)
		}
	}
	a.subscribeInbox(self)
}

func (a *Actor) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	//消息投递到nats
	case *castMsg:
		if err := publishTo(m.Module, m.ID, m.NodeId, m.Msg); err != nil {
			def.DLog.Info("natsrpc: cast [%s/%s/%s]: %v", m.Module, m.ID, m.NodeId, err)
		}
	//带回调的消息
	case *callMsg:
		a.handleCall(m)
	case *shutdownMsg:
		a.OnStop(ctx)
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	if a.timeWheel != nil {
		a.timeWheel.Stop()
	}
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
	go func() {
		for m := range ch {
			msg, err := a.codec.Unmarshal(m.Data)
			if err != nil {
				def.DLog.Info("natsrpc: unmarshal frame: %v", err)
				continue
			}
			tree.Default().Send(self, msg)
		}
	}()
	def.DLog.Info("natsrpc: subscribed %s (%d workers)", sub.subject, workers)
}

// subscribeInbox 订阅本 NatsActor 私有回包 inbox（Call 模式的回复统一进这里）。
func (a *Actor) subscribeInbox(self tree.PID) {

}

// handleCall 在 NatsActor goroutine 内执行：生成 reply inbox subject，存 pending，发消息。
func (a *Actor) handleCall(m *callMsg) {
}

package natsrpc

import (
	"log"

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

// Actor NATS 订阅 Actor，负责收消息、反序列化、投递。
type Actor struct {
	cfg  ActorConfig
	subs []*natsgo.Subscription
}

func NewActor(cfg ActorConfig) *Actor { return &Actor{cfg: cfg} }

func (a *Actor) OnInit(ctx actor.ActorContext) {
	self := ctx.Self()
	for _, sub := range a.cfg.Subs {
		switch sub.kind {
		case kindSub:
			a.subscribe(sub)
		case kindShutdown:
			a.subscribeShutdown(self, sub)
		}
	}
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *SendMsg:
		if err := send(m); err != nil {
			log.Printf("natsrpc: send [%s/%s]: %v", m.Module, m.NodeID, err)
		}
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

func (a *Actor) subscribe(sub SubConfig) {
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

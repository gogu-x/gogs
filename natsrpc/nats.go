package natsrpc

import (
	"fmt"
	"log"

	natsgo "github.com/nats-io/nats.go"

	actor "github.com/gogu-x/bigTree"
)

// ActorConfig 配置 Actor 的订阅行为。
type ActorConfig struct {
	GameIn     string                         // 订阅 gate.in.{GameIn}
	GateOut    string                         // 订阅 gate.out.{GateOut}.*
	LookupConn func(uint64) (actor.PID, bool) // gate 模式：connID → ConnActor PID
	Shutdown   struct{ ServerID, InstID string }
}

// Actor 通用 NATS Actor，HandleMessage 完全由内部 Router 驱动。
type Actor struct {
	cfg    ActorConfig
	router actor.Router
	subs   []*natsgo.Subscription
}

func NewActor(cfg ActorConfig) *Actor {
	return &Actor{cfg: cfg}
}

// ─── 生命周期 ─────────────────────────────────────────────────────────────────

func (a *Actor) OnInit(ctx actor.ActorContext) {
	a.router.Register(&InboundMsg{}, a.handleInbound)
	a.router.Register(&OutboundMsg{}, a.handleOutbound)
	a.router.Register(&rawMsg{}, a.handleRaw)
	a.router.Register(&ReplyMsg{}, a.handleReply)
	a.router.Register(&shutdownMsg{}, a.handleShutdown)

	self := ctx.Self()

	if a.cfg.GameIn != "" {
		sub, err := nc.Subscribe(fmt.Sprintf(subGameIn, a.cfg.GameIn), func(m *natsgo.Msg) {
			a.onGameInMsg(self, m)
		})
		if err != nil {
			log.Fatalf("natsrpc: subscribe GameIn error: %v", err)
		}
		a.subs = append(a.subs, sub)
		log.Printf("natsrpc: subscribed gate.in.%s", a.cfg.GameIn)
	}

	if a.cfg.GateOut != "" {
		ch := make(chan *natsgo.Msg, 65536)
		sub, err := nc.ChanSubscribe(fmt.Sprintf("gate.out.%s.*", a.cfg.GateOut), ch)
		if err != nil {
			log.Fatalf("natsrpc: subscribe GateOut error: %v", err)
		}
		a.subs = append(a.subs, sub)
		go a.runGateOutWorker(self, ch)
		log.Printf("natsrpc: subscribed gate.out.%s.*", a.cfg.GateOut)
	}

	if a.cfg.Shutdown.ServerID != "" {
		sub, err := nc.Subscribe(
			fmt.Sprintf(subGameShutdown, a.cfg.Shutdown.ServerID, a.cfg.Shutdown.InstID),
			func(_ *natsgo.Msg) { actor.Send(self, &shutdownMsg{}) },
		)
		if err != nil {
			log.Printf("natsrpc: subscribe shutdown error: %v", err)
		} else {
			a.subs = append(a.subs, sub)
		}
	}
}

func (a *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *Actor) OnStop(_ actor.ActorContext) {
	for _, sub := range a.subs {
		_ = sub.Unsubscribe()
	}
	actor.Default().Shutdown()
}

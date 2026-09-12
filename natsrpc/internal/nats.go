package internal

import (
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	natsgo "github.com/nats-io/nats.go"
)

const (
	natsRequestTimeout       = timer.TimerType(1)
	natsPendingMessageLimit  = 64 * 1024
	natsPendingByteLimit     = 64 * 1024 * 1024
	natsActorMailboxCapacity = 8 * 1024
)

// publisher is the minimal NATS transport needed by the send path. Keeping it
// small permits deterministic unit tests without opening a network connection.
type publisher interface {
	Publish(subject string, data []byte) error
}

type stoppable interface {
	Stop()
}

// Nats subscribes to one module/server/node subject, routes inbound messages,
// and owns pending request timeout state.
type Nats struct {
	typ       string
	serverId  int
	nodeId    int
	nastSub   *natsgo.Subscription
	timeWheel *timer.TimeWheel
	codec     codec.Codec
	system    *tree.Tree
	conn      *natsgo.Conn
	transport publisher
	natsURl   string

	// ReqMsgMap contains requests awaiting ACK. It remains exported for
	// compatibility with existing code.
	ReqMsgMap map[int64]*tree.Envelope

	requestTimers map[int64]stoppable
	afterTimeout  func(time.Duration, int64) stoppable
	nextRequestID atomic.Int64
}

func init() {
	// NatsMsgNtf already exists in generated code but is deliberately registered
	// here rather than changing generated pb/register.go.
	codec.RegisterMsg(&sspb.NatsMsgNtf{})
}

func NewActor(typ string, serverId int, nodeId int, url string) *Nats {
	return &Nats{
		typ:           typ,
		serverId:      serverId,
		nodeId:        nodeId,
		codec:         codec.ProtoCodec,
		natsURl:       url,
		ReqMsgMap:     make(map[int64]*tree.Envelope),
		requestTimers: make(map[int64]stoppable),
	}
}

// Name implements tree.Actor.
func (ns *Nats) Name() string { return def.Nats }

// MailboxSize absorbs short NATS bursts (for example, a complete battle
// report) while preserving actor ordering and backpressure.
func (ns *Nats) MailboxSize() int { return natsActorMailboxCapacity }

func (ns *Nats) OnInit(ctx tree.Context) {
	nc, err := natsgo.Connect(ns.natsURl)
	if err != nil {
		panic(err)
	}
	ns.conn = nc
	ns.transport = nc
	ns.system = ctx.System()
	ns.timeWheel = timer.NewTimeWheel(1024, ctx.Self(), ns.system)
	ns.timeWheel.Register(natsRequestTimeout, func(data interface{}) {
		sessionID, ok := data.(int64)
		if ok {
			ns.timeoutRequest(sessionID)
		}
	})
	ns.afterTimeout = func(d time.Duration, sessionID int64) stoppable {
		return ns.timeWheel.After(natsRequestTimeout, d, sessionID)
	}
	ns.subscribe(ctx, Subject(ns.typ, ns.serverId, ns.nodeId))
}

func (ns *Nats) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *NatsMsg:
		if m.Cast {
			err := ns.catsMsg(m)
			ctx.Response(nil, err)
			if err != nil {
				tlog.Log.Error("%v", err)
			}
		} else {
			ns.handleCall(ctx, m)
		}
	case *sspb.NatsMsgReq:
		ns.NatsMsgReq(ctx, m)
	case *sspb.NatsMsgAck:
		ns.NatsMsgAck(ctx, m)
	case *sspb.NatsMsgNtf:
		ns.NatsMsgNtf(ctx, m)
	default:
		ns.MsgHandle(ctx, msg)
	}
}

func (ns *Nats) OnStop(_ tree.Context) {
	if ns.timeWheel != nil {
		ns.timeWheel.Stop()
	}
	for sessionID := range ns.ReqMsgMap {
		ns.finishPending(sessionID, nil, errors.New("natsrpc: actor stopped"))
	}
	if ns.nastSub != nil {
		if err := ns.nastSub.Unsubscribe(); err != nil {
			tlog.Log.Error("natsrpc: unsubscribe: %v", err)
		}
	}
}

func (ns *Nats) publish(subject string, data []byte) error {
	if ns.transport == nil {
		return errors.New("natsrpc: publish before initialization")
	}
	return ns.transport.Publish(subject, data)
}

func (ns *Nats) subscribe(ctx tree.Context, sub string) {
	if ns.conn == nil {
		panic("natsrpc: subscribe before connection initialization")
	}
	s, err := ns.conn.SubscribeSync(sub)
	if err != nil {
		log.Printf("natsrpc: subscribe %s: %v", sub, err)
		panic(fmt.Sprintf("natsrpc: subscribe %s: %v", sub, err))
	}
	if err := s.SetPendingLimits(natsPendingMessageLimit, natsPendingByteLimit); err != nil {
		_ = s.Unsubscribe()
		log.Printf("natsrpc: set pending limits for %s: %v", sub, err)
		panic(fmt.Sprintf("natsrpc: set pending limits for %s: %v", sub, err))
	}
	ns.nastSub = s
	go ns.consumeSubscription(ctx, sub, s)
	tlog.Log.Info("natsrpc: subscribed %s (pending_messages=%d pending_bytes=%d)",
		sub, natsPendingMessageLimit, natsPendingByteLimit)
}

// consumeSubscription is the single ordered ingress path. SubscribeSync keeps
// bursts in the NATS subscription pending queue instead of dropping as soon as
// a small user channel fills. ctx.Send intentionally retains actor backpressure;
// the enlarged pending queue and Nats mailbox absorb finite battle-report bursts.
func (ns *Nats) consumeSubscription(ctx tree.Context, sub string, s *natsgo.Subscription) {
	for {
		m, err := s.NextMsg(time.Second)
		if err != nil {
			if errors.Is(err, natsgo.ErrTimeout) {
				continue
			}
			if !s.IsValid() {
				return
			}
			tlog.Log.Error("natsrpc: receive frame from %s: %v", sub, err)
			continue
		}
		msg, err := ns.codec.Unmarshal(m.Data)
		if err != nil {
			tlog.Log.Info("natsrpc: unmarshal frame from %s: %v", sub, err)
			continue
		}
		if !ctx.Send(ctx.Self(), msg) {
			tlog.Log.Info("natsrpc: stop consuming %s: actor stopped", sub)
			return
		}
	}
}

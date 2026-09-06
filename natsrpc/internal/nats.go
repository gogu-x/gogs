package internal

import (
	"log"
	"sync/atomic"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const natsRequestTimeout = timer.TimerType(1)

// Nats NATS 订阅 Nats，负责收消息、反序列化、投递，并承载 Cast/Call 的发送与超时管理。
type Nats struct {
	typ       string
	serverId  int
	nodeId    int
	nastSub   *natsgo.Subscription
	timeWheel *timer.TimeWheel
	codec     codec.Codec
	system    *tree.Tree
	conn      *natsgo.Conn
	natsURl   string

	//ReqMsgMap 发送者消息
	ReqMsgMap map[int64]*tree.Envelope

	nextRequestID atomic.Int64
}

func NewActor(typ string, serverId int, nodeId int, url string) *Nats {
	return &Nats{
		typ:       typ,
		serverId:  serverId,
		nodeId:    nodeId,
		codec:     codec.ProtoCodec,
		natsURl:   url,
		ReqMsgMap: make(map[int64]*tree.Envelope),
	}
}

// Name 实现 tree.Actor，注册名默认为 constant.ActorNats。
func (ns *Nats) Name() string {
	return def.Nats
}

func (ns *Nats) OnInit(ctx tree.Context) {
	nc, err := natsgo.Connect(ns.natsURl)
	if err != nil {
		panic(err)
	}
	ns.conn = nc
	ns.system = ctx.System()
	ns.timeWheel = timer.NewTimeWheel(1024, ctx.Self(), ns.system)
	ns.timeWheel.Register(natsRequestTimeout, func(data interface{}) {})
	//订阅消息
	ns.subscribe(ctx, Subject(ns.typ, ns.serverId, ns.nodeId))
}

func (ns *Nats) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	//消息投递到nats
	case *CastMsg:
		ns.catsMsg(m)
	//带回调的消息
	case *CallMsg:
		ns.handleCall(ctx, m)
	case *sspb.NatsMsgReq:
		ns.NatsMsgReq(ctx, m)
	default:
		ns.MsgHandle(ctx, msg)
	}
}

func (ns *Nats) OnStop(_ tree.Context) {
	if ns.timeWheel != nil {
		ns.timeWheel.Stop()
	}
	ns.nastSub.Unsubscribe()
}

func (ns *Nats) subscribe(ctx tree.Context, sub string) {
	ch := make(chan *natsgo.Msg, 128)

	s, err := ns.conn.ChanSubscribe(sub, ch)
	if err != nil {
		log.Fatalf("natsrpc: subscribe %s: %v", sub, err)
	}
	ns.nastSub = s
	go func() {
		for m := range ch {
			msg := &sspb.NatsMsgReq{}
			err := proto.Unmarshal(m.Data, msg)
			if err != nil {
				tlog.Log.Info("natsrpc: unmarshal frame from %s: %v", sub, err)
				continue
			}
			tlog.Log.Info("natsrpc msg=%v", msg)
			ctx.Send(ctx.Self(), msg)
		}
	}()
	tlog.Log.Info("natsrpc: subscribed %s ", sub)
}

package player

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/player/internal"
	"github.com/gogu-x/gogs/natsrpc"
)

// PlayerActor 每个在线玩家独立一个 Actor
type PlayerActor struct {
	uid    uint64
	connID uint64
	router actor.Router
	s      *internal.Session
}

func NewPlayerActor(uid, connID uint64) *PlayerActor {
	return &PlayerActor{uid: uid, connID: connID}
}

func (p *PlayerActor) OnInit(ctx actor.ActorContext) {
	ctx.Register(constant.PlayerName(p.uid))
	data := internal.Load(p.uid)
	p.s = internal.NewSession(data)
	internal.InitRoutes(&p.router, p.s)

	p.router.Register(&natsrpc.Frame{}, func(ctx actor.ActorContext, msg interface{}) {
		frame := msg.(*natsrpc.Frame)
		if frame.MsgType == natsrpc.MsgTypeDisconnect {
			log.Printf("PlayerActor[%d]: client disconnected, stopping", p.uid)
			ctx.Stop()
			return
		}
		p.s.ConnID = frame.ConnId
		p.s.GateId = frame.GateId
		inner, err := codec.ProtoCodec.Unmarshal(frame.Payload)
		if err != nil {
			log.Printf("PlayerActor[%d]: unmarshal payload: %v", p.uid, err)
			return
		}
		p.router.Route(ctx, inner)
	})
}

func (p *PlayerActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	p.router.Route(ctx, msg)
}

func (p *PlayerActor) OnStop(_ actor.ActorContext) {
	log.Printf("PlayerActor[%d]: saving to DB...", p.uid)
	if err := p.s.Data.Save(); err != nil {
		log.Printf("PlayerActor[%d]: save error: %v", p.uid, err)
	}
}

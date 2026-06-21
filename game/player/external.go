package player

import (
	"errors"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/player/internal"
	"github.com/gogu-x/gogs/natsrpc"
	"go.mongodb.org/mongo-driver/v2/mongo"
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

	internal.Load(ctx, p.uid, func(data *internal.PlayerData, err error) {
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				// 新玩家
				data = internal.NewPlayerData(p.uid)
				data.Save()
			} else {
				log.Printf("PlayerActor[%d]: load failed: %v", p.uid, err)
				ctx.Stop()
				return
			}
		}
		p.s = internal.NewSession(data, ctx)
		internal.InitRoutes(&p.router, p.s)
		internal.InitTimers(p.s)
		log.Printf("PlayerActor[%d]: ready", p.uid)
	})

	// 注册 Frame handler（Load 未完成时消息在 mailbox 缓冲，回调执行后路由生效）
	p.router.Register(&natsrpc.Frame{}, func(ctx actor.ActorContext, msg interface{}) {
		frame := msg.(*natsrpc.Frame)
		//if frame.MsgType == natsrpc.MsgTypeDisconnect {
		//	log.Printf("PlayerActor[%d]: client disconnected, stopping", p.uid)
		//	ctx.Stop()
		//	return
		//}
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
	p.s.Data.Save()
}

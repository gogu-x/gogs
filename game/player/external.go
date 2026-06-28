package player

import (
	"errors"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/player/internal"
	"github.com/gogu-x/gogs/game/player/internal/base"
	"github.com/gogu-x/gogs/natsrpc"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Player 每个在线玩家独立一个 Actor
type Player struct {
	uid    uint64
	connID uint64
	router actor.Router
	s      *base.Session
}

func NewPlayerActor(uid, connID uint64) *Player {
	return &Player{uid: uid, connID: connID}
}

func (p *Player) OnInit(ctx actor.ActorContext) {
	ctx.Register(constant.PlayerName(p.uid))

	// 同步加载玩家数据：阻塞当前 PlayerActor goroutine 直到完成或超时。
	// 框架在 OnInit 返回后才开始消费 mailbox，因此 OnInit 返回时 p.s 必已就绪，
	// 任何 Frame 都不可能在 Session 初始化之前被处理，从根上消除空指针竞态。
	data, err := base.Load(ctx, p.uid)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			data = base.NewPlayerData(p.uid)
		} else {
			log.Printf("PlayerActor[%d]: load failed: %v", p.uid, err)
			ctx.Stop()
			return
		}
	}
	p.s = base.NewSession(data, ctx)
	internal.InitRoutes(&p.router, p.s)
	base.InitTimers(p.s)
	log.Printf("PlayerActor[%d]: ready", p.uid)

	// 注册 Frame handler。此时 p.s 已就绪。
	p.router.Register(&natsrpc.Frame{}, func(ctx actor.ActorContext, msg interface{}) {
		frame := msg.(*natsrpc.Frame)
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

func (p *Player) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	p.router.Route(ctx, msg)
}

func (p *Player) OnStop(_ actor.ActorContext) {
	if p.s == nil {
		log.Printf("PlayerActor[%d]: Session nil", p.uid)
		return
	}
	p.s.Data.Save()
}

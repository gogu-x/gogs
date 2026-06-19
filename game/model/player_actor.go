package model

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/game/router"
	"github.com/gogu-x/gogs/game/timer"
	"github.com/gogu-x/gogs/natsrpc"
)

// PlayerActor 每个在线玩家独立一个 Actor，持有该玩家的完整上下文
type PlayerActor struct {
	connID uint64
	uid    uint64
	router actor.Router
	app    *app.App
}

func newPlayerActor(uid, connID uint64) *PlayerActor {
	return &PlayerActor{
		uid:    uid,
		connID: connID,
	}
}

func (p *PlayerActor) OnInit(ctx actor.ActorContext) {
	ctx.Register(constant.PlayerName(p.uid))
	p.app = app.New(p.uid)
	router.Init(&p.router, p.app)
	timer.Init(ctx)

	// 注册 InboundMsg：更新连接上下文，再按业务消息类型路由
	p.router.Register(&natsrpc.InboundMsg{}, func(ctx actor.ActorContext, msg interface{}) {
		m := msg.(*natsrpc.InboundMsg)
		p.app.ConnID = m.ConnID
		p.app.GateId = m.GateID
		p.router.Route(ctx, m.Msg)
	})
}

func (p *PlayerActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	p.router.Route(ctx, msg)
}

func (p *PlayerActor) OnStop(ctx actor.ActorContext) {
	// TODO: 将玩家内存数据持久化到 DB
	log.Printf("PlayerActor[%d]: saving to DB...", p.uid)
}

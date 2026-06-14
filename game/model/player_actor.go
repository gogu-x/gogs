package model

import (
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/router"
	"github.com/gogu-x/gogs/game/timer"
)

// playerActorName 返回玩家 Actor 的注册名，格式 player-{uid}
func playerActorName(uid uint64) string {
	return fmt.Sprintf("player-%d", uid)
}

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
	ctx.Register(playerActorName(p.uid))
	p.app = app.New(p.uid)
	router.Init(&p.router, p.app)
	timer.Init(ctx)
}

func (p *PlayerActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *inboundMsg:
		p.app.ConnID = m.connID
		p.app.GateId = m.gateId
		p.router.Route(ctx, m.msg)
	}
}

func (p *PlayerActor) OnStop(ctx actor.ActorContext) {
	// TODO: 将玩家内存数据持久化到 DB
	log.Printf("PlayerActor[%d]: saving to DB...", p.uid)
}

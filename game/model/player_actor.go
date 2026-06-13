package model

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/bigTree/timer"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/router"
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
	timer  *timer.TimeWheel
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
	p.timer = timer.NewTimeWheel(10240)
	router.Init(&p.router, p.app)
}

func (p *PlayerActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *inboundMsg:
		p.app.ConnID = m.connID
		p.router.Route(ctx, m.msg)
	}
}

func (p *PlayerActor) OnStop(ctx actor.ActorContext) {
	if p.timer != nil {
		p.timer.Stop()
	}
}

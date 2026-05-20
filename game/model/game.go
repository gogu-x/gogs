package model

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/bigTree/timer"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/router"
)

type GameActor struct {
	Router actor.Router
	app    *app.App
	timer  *timer.TimeWheel
}

func (g *GameActor) OnInit(ctx actor.ActorContext) {
	g.timer = timer.NewTimeWheel(10240)
	g.app = app.New()
	router.Init(&g.Router, g.app)

}

func (g *GameActor) OnStop(ctx actor.ActorContext) {
	if g.timer != nil {
		g.timer.Stop()
	}
}

func (g *GameActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	message := msg.(*inboundMsg)
	g.Router.Route(app.WrapContext(ctx, message.uid, message.connID), message.msg)
}

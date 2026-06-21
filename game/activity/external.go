package activity

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/activity/internal"
)

type ActivityActor struct {
	mgr    *internal.Mgr
	router actor.Router
}

// NewActivityActor 创建 ActivityActor
func NewActivityActor() *ActivityActor {
	return &ActivityActor{mgr: internal.NewMgr()}
}

func (a *ActivityActor) OnInit(_ actor.ActorContext) {
	internal.InitRoutes(&a.router, a.mgr)
}

func (a *ActivityActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *ActivityActor) OnStop(_ actor.ActorContext) {}

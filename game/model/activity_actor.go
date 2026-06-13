package model

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/activity"
)

type ActivityActor struct {
	store  *activity.ActivityMgr
	router actor.Router
}

func NewActivityActor() *ActivityActor {
	return &ActivityActor{store: activity.NewActivityMgr()}
}

func (a *ActivityActor) OnInit(_ actor.ActorContext) {
	activity.InitRoutes(&a.router, a.store)
}

func (a *ActivityActor) OnStop(_ actor.ActorContext) {}

func (a *ActivityActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	a.router.Route(ctx, msg)
}

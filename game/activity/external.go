package activity

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/activity/internal"
	"github.com/gogu-x/tree"
)

type ActivityActor struct {
	mgr     *internal.Mgr
	router  tree.Router
	context tree.Context
}

// NewActivityActor 创建 ActivityActor
func NewActivityActor() *ActivityActor {
	return &ActivityActor{mgr: internal.NewMgr()}
}

func (a *ActivityActor) Name() string { return constant.ActorActivity }

func (a *ActivityActor) OnInit(c tree.Context) {
	a.context = c
	internal.InitRoutes(&a.router, a.mgr)
}

func (a *ActivityActor) HandleMessage(ctx tree.Context, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *ActivityActor) OnStop(_ tree.Context) {}

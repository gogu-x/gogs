package activity

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity/internal"
	"github.com/gogu-x/tree"
)

type Activity struct {
	mgr     *internal.Mgr
	router  tree.Router
	context tree.Context
}

// NewActivity 创建 Activity
func NewActivity() *Activity {
	return &Activity{mgr: internal.NewMgr()}
}

func (a *Activity) Name() string { return def.ActorActivity }

func (a *Activity) OnInit(c tree.Context) {
	a.context = c
	internal.InitRoutes(&a.router, a.mgr)
}

func (a *Activity) HandleMessage(ctx tree.Context, msg interface{}) {
	a.router.Route(ctx, msg)
}

func (a *Activity) OnStop(_ tree.Context) {}

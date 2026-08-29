package guild

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/guild/internal"
	"github.com/gogu-x/tree"
)

type Guild struct {
	store  *internal.Store
	router tree.Router
}

// NewGuild 创建 Guild
func NewGuild() *Guild {
	return &Guild{store: internal.NewStore()}
}

func (g *Guild) Name() string { return constant.Guild }

func (g *Guild) OnInit(_ tree.Context) {
	internal.InitRoutes(&g.router, g.store)
}

func (g *Guild) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *Guild) OnStop(_ tree.Context) {}

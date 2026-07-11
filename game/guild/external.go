package guild

import (
	"github.com/gogu-x/gogs/game/guild/internal"
	"github.com/gogu-x/tree"
)

type GuildActor struct {
	store  *internal.Store
	router tree.Router
}

// NewGuildActor 创建 GuildActor
func NewGuildActor() *GuildActor {
	return &GuildActor{store: internal.NewStore()}
}

func (g *GuildActor) OnInit(_ tree.Context) {
	internal.InitRoutes(&g.router, g.store)
}

func (g *GuildActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GuildActor) OnStop(_ tree.Context) {}

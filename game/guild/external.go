package guild

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/guild/internal"
)

type GuildActor struct {
	store  *internal.Store
	router actor.Router
}

// NewGuildActor 创建 GuildActor
func NewGuildActor() *GuildActor {
	return &GuildActor{store: internal.NewStore()}
}

func (g *GuildActor) OnInit(_ actor.ActorContext) {
	internal.InitRoutes(&g.router, g.store)
}

func (g *GuildActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GuildActor) OnStop(_ actor.ActorContext) {}

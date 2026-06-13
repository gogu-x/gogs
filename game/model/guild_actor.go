package model

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/guild"
)

type GuildActor struct {
	store  *guild.Store
	router actor.Router
}

func NewGuildActor() *GuildActor {
	return &GuildActor{store: guild.NewStore()}
}

func (g *GuildActor) OnInit(_ actor.ActorContext) {
	guild.InitRoutes(&g.router, g.store)
}

func (g *GuildActor) OnStop(_ actor.ActorContext) {}

func (g *GuildActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	g.router.Route(ctx, msg)
}

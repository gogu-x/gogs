package guild

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func InitRoutes(r *actor.Router, s *Store) {
	app.Register(r, &protoGuild.CreateGuildReq{}, s.Create)
	app.Register(r, &protoGuild.JoinGuildReq{}, s.Join)
	app.Register(r, &protoGuild.LeaveGuildReq{}, s.Leave)
	app.Register(r, &protoGuild.GetGuildReq{}, s.Get)
}

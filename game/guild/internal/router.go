package internal

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/util"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func InitRoutes(r *actor.Router, s *Store) {
	util.Register(r, &protoGuild.CreateGuildReq{}, s.Create)
	util.Register(r, &protoGuild.JoinGuildReq{}, s.Join)
	util.Register(r, &protoGuild.LeaveGuildReq{}, s.Leave)
	util.Register(r, &protoGuild.GetGuildReq{}, s.Get)
}

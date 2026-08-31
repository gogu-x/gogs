package internal

import (
	"github.com/gogu-x/gogs/game/util"
	"github.com/gogu-x/gogs/pb/pb_guild"
	"github.com/gogu-x/tree"
)

func InitRoutes(r *tree.Router, s *Store) {
	util.Register(r, &pb_guild.CreateGuildReq{}, s.Create)
	util.Register(r, &pb_guild.JoinGuildReq{}, s.Join)
	util.Register(r, &pb_guild.LeaveGuildReq{}, s.Leave)
	util.Register(r, &pb_guild.GetGuildReq{}, s.Get)
}

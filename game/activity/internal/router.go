package internal

import (
	"github.com/gogu-x/gogs/game/util"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/tree"
)

func InitRoutes(r *tree.Router, s *Mgr) {
	util.Register(r, &pb_activity.GetActivityListReq{}, s.GetList)
}

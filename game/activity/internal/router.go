package internal

import (
	"github.com/gogu-x/gogs/game/util"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/tree"
)

func InitRoutes(r *tree.Router, s *Mgr) {
	util.Register(r, &pb_activity.GetActivityListReq{}, s.GetList)
	util.Register(r, &pb_activity.JoinActivityReq{}, s.Join)
	util.Register(r, &pb_activity.GetProgressReq{}, s.GetProgress)
	util.Register(r, &pb_activity.ClaimRewardReq{}, s.ClaimReward)
}

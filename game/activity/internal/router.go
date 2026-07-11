package internal

import (
	"github.com/gogu-x/gogs/game/util"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/tree"
)

func InitRoutes(r *tree.Router, s *Mgr) {
	util.Register(r, &protoActivity.GetActivityListReq{}, s.GetList)
	util.Register(r, &protoActivity.JoinActivityReq{}, s.Join)
	util.Register(r, &protoActivity.GetProgressReq{}, s.GetProgress)
	util.Register(r, &protoActivity.ClaimRewardReq{}, s.ClaimReward)
}

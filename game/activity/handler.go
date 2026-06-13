package activity

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoActivity"
)

func InitRoutes(r *actor.Router, s *ActivityMgr) {
	app.Register(r, &protoActivity.GetActivityListReq{}, s.GetList)
	app.Register(r, &protoActivity.JoinActivityReq{}, s.Join)
	app.Register(r, &protoActivity.GetProgressReq{}, s.GetProgress)
	app.Register(r, &protoActivity.ClaimRewardReq{}, s.ClaimReward)
}

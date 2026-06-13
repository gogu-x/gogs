package activity

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoActivity"
)

func InitRoutes(r *actor.Router, s *ActivityMgr) {
	app.Handle(r, &protoActivity.GetActivityListReq{}, s.GetList)
	app.Handle(r, &protoActivity.JoinActivityReq{}, s.Join)
	app.Handle(r, &protoActivity.GetProgressReq{}, s.GetProgress)
	app.Handle(r, &protoActivity.ClaimRewardReq{}, s.ClaimReward)
}

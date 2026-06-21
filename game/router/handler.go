package router

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/ctl"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func Init(r *actor.Router, a *app.App) {
	r.Register(&protoGateway.LoginReq{}, a.Handle(ctl.AutoLogin))
	r.Register(&protoGateway.RegisterReq{}, a.Handle(ctl.AutoRegister))

	r.Register(&protoChat.ChatReq{}, a.Handle(ctl.ChatService))

	r.Register(&protoGuild.CreateGuildReq{}, a.Handle(ctl.CreateGuild))
	r.Register(&protoGuild.JoinGuildReq{}, a.Handle(ctl.JoinGuild))
	r.Register(&protoGuild.LeaveGuildReq{}, a.Handle(ctl.LeaveGuild))
	r.Register(&protoGuild.GetGuildReq{}, a.Handle(ctl.GetGuild))

	r.Register(&protoActivity.GetActivityListReq{}, a.Handle(ctl.GetActivityList))
	r.Register(&protoActivity.JoinActivityReq{}, a.Handle(ctl.JoinActivity))
	r.Register(&protoActivity.GetProgressReq{}, a.Handle(ctl.GetProgress))
	r.Register(&protoActivity.ClaimRewardReq{}, a.Handle(ctl.ClaimReward))
}

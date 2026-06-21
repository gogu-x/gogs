package internal

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func InitRoutes(r *actor.Router, s *Session) {
	r.Register(&protoGateway.LoginReq{}, s.Handle(AutoLogin))
	r.Register(&protoGateway.RegisterReq{}, s.Handle(AutoRegister))
	r.Register(&protoChat.ChatReq{}, s.Handle(ChatService))

	r.Register(&protoGuild.CreateGuildReq{}, s.Handle(CreateGuild))
	r.Register(&protoGuild.JoinGuildReq{}, s.Handle(JoinGuild))
	r.Register(&protoGuild.LeaveGuildReq{}, s.Handle(LeaveGuild))
	r.Register(&protoGuild.GetGuildReq{}, s.Handle(GetGuild))

	r.Register(&protoActivity.GetActivityListReq{}, s.Handle(GetActivityList))
	r.Register(&protoActivity.JoinActivityReq{}, s.Handle(JoinActivity))
	r.Register(&protoActivity.GetProgressReq{}, s.Handle(GetProgress))
	r.Register(&protoActivity.ClaimRewardReq{}, s.Handle(ClaimReward))
}

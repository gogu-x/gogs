package guild

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func InitRoutes(r *actor.Router, s *Store) {
	app.Handle(r, &protoGuild.CreateGuildReq{}, s.Create)
	app.Handle(r, &protoGuild.JoinGuildReq{}, s.Join)
	app.Handle(r, &protoGuild.LeaveGuildReq{}, s.Leave)
	app.Handle(r, &protoGuild.GetGuildReq{}, s.Get)

	r.Register(&UpdateMemberMsg{}, func(_ actor.ActorContext, msg interface{}) {
		m := msg.(*UpdateMemberMsg)
		s.UpdateMember(m.UID, m.Name, m.Level)
	})
}

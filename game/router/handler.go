package router

import (
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/ctl"
	pb "github.com/gogu-x/gogs/pb/game"

	actor "github.com/gogu-x/bigTree"
)

func Init(r *actor.Router, a *app.App) {
	r.Register(&pb.ChatReq{}, a.Handle(ctl.ChatService))
}

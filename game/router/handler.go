package router

import (
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/ctl"
	"github.com/gogu-x/gogs/pb/chat"

	actor "github.com/gogu-x/bigTree"
)

func Init(r *actor.Router, a *app.App) {
	r.Register(&chat.ChatReq{}, a.Handle(ctl.ChatService))
}

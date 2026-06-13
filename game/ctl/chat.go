package ctl

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/protoChat"
)

func ChatService(a *app.App, _ actor.ActorContext, msg interface{}) {
	req := msg.(*protoChat.ChatReq)
	fmt.Printf("game server [%d] player says: %s\n", config.ServerID, req.Content)
	a.Reply(&protoChat.ChatAck{State: 2})
}

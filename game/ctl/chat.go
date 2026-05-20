package ctl

import (
	"fmt"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/pb/chat"
)

func ChatService(ctx *app.Context, msg interface{}) {
	req := msg.(*chat.ChatReq)

	fmt.Printf("game server [%d] player %s says: %s\n", config.ServerID, req.Content)
	ctx.Reply(&chat.ChatReq{Content: "game server ChatService"})
}

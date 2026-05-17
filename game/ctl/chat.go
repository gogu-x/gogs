package ctl

import (
	"fmt"

	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/app"
	pb "github.com/gogu-x/gogs/pb/game"
)

func ChatService(ctx *app.Context, msg interface{}) {
	req := msg.(*pb.ChatReq)

	fmt.Printf("game server [%d] player %s says: %s\n", config.ServerID, req.Content)
	ctx.Reply(&pb.ChatResp{Content: "game server ChatService"})
}

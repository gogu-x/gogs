package model

import (
	"fmt"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"google.golang.org/protobuf/proto"
)

func init() {
	// 兜底：将所有解码后的消息路由到对应 PlayerActor
	natsrpc.SetFallback(func(ctx actor.ActorContext, msg proto.Message, meta natsrpc.Meta) {
		name := constant.PlayerName(meta.UID)
		pid, exists := ctx.Lookup(name)
		if !exists {
			pid = actor.Spawn(name, newPlayerActor(meta.UID, meta.ConnID))
		}
		ctx.Send(pid, &natsrpc.InboundMsg{Msg: msg, UID: meta.UID, ConnID: meta.ConnID, GateID: meta.GateID})
	})
}

func NewNatsActor(instID string) *natsrpc.Actor {
	return natsrpc.NewActor(natsrpc.ActorConfig{
		GameIn: fmt.Sprintf("%d", config.ServerID),
		Shutdown: struct{ ServerID, InstID string }{
			ServerID: fmt.Sprintf("%d", config.ServerID),
			InstID:   instID,
		},
	})
}

package service

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/natsrpc"
	"google.golang.org/protobuf/proto"
)

type NATSNotifier struct{}

func (NATSNotifier) Notify(source Source, msg proto.Message) error {
	return natsrpc.Cast(natsrpc.Game, def.BattleClient, source.ServerID, source.NodeID, msg)
}

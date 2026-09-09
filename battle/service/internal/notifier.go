package internal

import (
	"github.com/gogu-x/gogs/battle/battle/internal"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/natsrpc"
	"google.golang.org/protobuf/proto"
)

// NATSNotifier 实现 battle.Notifier：把生命周期通知 Cast 回投给发起方 Game 节点
// 上的 BATTLE_CLIENT battle。由装配层注入 per-battle battle。
type NATSNotifier struct{}

func (NATSNotifier) Notify(source internal.Source, msg proto.Message) error {
	return natsrpc.Cast(natsrpc.Game, def.BattleClient, source.ServerID, source.NodeID, msg)
}

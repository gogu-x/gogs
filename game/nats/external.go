package nats

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/tree"
)

// NewNats 创建 game 进程的 NATS 订阅 Actor：
// 订阅 game:{serverID}:{nodeID}，所有 Frame 投给 Play Actor，
// 同时订阅本节点的关闭信号。
func NewNats(serverID, nodeID string) *natsrpc.Actor {
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GameInSubject(serverID, nodeID), RouteToPlay, 8),
			natsrpc.ShutdownSub(serverID, nodeID),
		},
	})
}

// RouteToPlay 玩家 Frame 统一投给 Play Actor（玩家数据常驻该 Actor，串行处理）。
func RouteToPlay(_ *natsrpc.Frame) (tree.PID, bool) {
	return tree.Default().Lookup(constant.PLAY)
}

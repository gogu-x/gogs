package nats

import (
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/natsrpc"
)

// NewNats 创建 game 进程的 NATS 订阅 Actor：
// 订阅 game:{serverID}:{nodeID}，所有 Frame 投给 Play Actor，
// 同时订阅本节点的关闭信号。
func NewNats(serverID, nodeID uint32) *natsrpc.Actor {
	return natsrpc.NewActor(natsrpc.ActorConfig{
		Subs: []natsrpc.SubConfig{
			natsrpc.Sub(natsrpc.GameSubject(serverID, nodeID), constant.PLAY, 1),
			natsrpc.Sub(natsrpc.ActivitySubject(serverID, nodeID), constant.ActorActivity, 1),
		},
	})
}

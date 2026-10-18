package play

import (
	"github.com/gogu-x/gogs/game/play/internal"
	"github.com/gogu-x/tree"
)

// NewPlayerActor 创建并装配一个独占玩家数据的角色。
func NewPlayerActor(uid uint64) tree.Actor {
	return internal.NewPlayerActor(uid)
}

// NewPlayerActorWithMessages 创建带有预先排队系统消息的玩家角色。
func NewPlayerActorWithMessages(uid uint64, messages []interface{}) tree.Actor {
	return internal.NewPlayerActorWithMessages(uid, messages)
}

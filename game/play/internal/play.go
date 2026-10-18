// internal 包装配单玩家角色的路由和事件监听器。
package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
)

// NewPlayerActor 构造一个由玩家编号独占的玩家角色。
func NewPlayerActor(uid uint64) *core.PlayerActor {
	return newPlayerActor(uid, nil)
}

// NewPlayerActorWithMessages 创建带有预先排队系统消息的玩家角色。
func NewPlayerActorWithMessages(uid uint64, messages []interface{}) *core.PlayerActor {
	return newPlayerActor(uid, append([]interface{}(nil), messages...))
}

func newPlayerActor(uid uint64, messages []interface{}) *core.PlayerActor {
	playerActor := &core.PlayerActor{
		UID: uid,
		Boot: func(playerActor *core.PlayerActor) {
			InitEvent(playerActor)
			InitRoutes(playerActor)
		},
	}
	playerActor.SetInitialMessages(messages)
	return playerActor
}

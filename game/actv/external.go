package actv

import (
	"github.com/gogu-x/gogs/game/actv/internal/actor"
)

// NewActivity 创建指定服务器编号对应的全局活动服务。
func NewActivity() *actor.Activity {
	return actor.NewActivity()
}

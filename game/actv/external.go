package actv

import (
	"github.com/gogu-x/gogs/game/actv/internal/actor"
)

// NewActv 创建指定服务器编号对应的全局活动服务。
func NewActv() *actor.Actv {
	return actor.NewActv()
}

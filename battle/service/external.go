package service

import (
	"github.com/gogu-x/gogs/battle/service/internal"
	"github.com/gogu-x/tree"
)

// New 创建战斗服务实例。
func New() tree.Actor {
	return internal.New()
}

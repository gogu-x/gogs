package play

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/tree"
)

// NewPlay 创建 play 模块的 Actor。
func NewPlay() tree.Actor {
	return &core.Play{}
}

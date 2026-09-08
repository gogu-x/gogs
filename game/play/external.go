package play

import (
	"github.com/gogu-x/gogs/game/play/internal"
	"github.com/gogu-x/tree"
)

// NewPlay 创建并完整装配 play 模块的 Actor。
func NewPlay() tree.Actor {
	return internal.NewPlay()
}

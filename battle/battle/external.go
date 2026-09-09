package battle

import (
	"github.com/gogu-x/gogs/battle/battle/internal"
	"github.com/gogu-x/tree"
)

func New() tree.Actor {
	return internal.NewBattleActor()
}

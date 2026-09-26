package internal

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/tree/db/mongorpc"
)

func RegDbLoader(play *core.Play) {
	lifecycle := mongorpc.NewLifecycle(mongorpc.NewStore(def.Mongo))
	play.Lifecycle = lifecycle
	lifecycle.Register(&player.PlayerMgr{})
}

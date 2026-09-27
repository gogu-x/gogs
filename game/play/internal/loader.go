package internal

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/tlog"
)

func RegDbLoader(play *core.Play) {
	lifecycle := mongorpc.NewLifecycle(
		mongorpc.NewStore(def.Mongo),
		play.TimeWheel,
		play.SystemContext(),
	)
	play.Lifecycle = lifecycle

	if err := lifecycle.Register(play.PlayerMgr); err != nil {
		tlog.Log.Error("[play/RegDbLoader] 注册玩家数据模型失败, err=%v", err)
		return
	}

}

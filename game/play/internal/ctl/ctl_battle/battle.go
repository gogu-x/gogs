package ctl_battle

import (
	"fmt"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battleclient"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	if !ctx.Cast(def.BattleClient, &battleclient.Begin{UID: req.UID, Input: req.Input}) {
		ctx.Response(nil, fmt.Errorf("play: battle client is unavailable"))
	}
}

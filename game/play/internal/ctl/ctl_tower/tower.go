package ctl_tower

import (
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

func OnTowerBattleResults(play *core.Play, arg *comm.Arg) {
	result, ok := arg.Get("settlement")
	if !ok {
		return
	}
	results, ok := result.(*battle.Settlement)
	if !ok {
		return
	}

	tlog.Log.Info("battle_result", result)

	if results.BattleType != pb_battle.BattleType_BATTLE_TYPE_TOWER {
		return
	}
	if results.Outcome != pb_battle.BattleOutcome_BATTLE_OUTCOME_ATTACKER_WIN {
		return
	}
	uid := results.Participants[0].UID
	player := play.PlayerMgr.Get(uid)
	if player == nil {
		return
	}
	player.TowerMgr.Layer += 1
}

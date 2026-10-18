package ctl_tower

import (
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// OnTowerBattleResults 更新当前玩家的爬塔进度。
func OnTowerBattleResults(actor *core.PlayerActor, arg *comm.Arg) {
	if actor == nil || arg == nil {
		return
	}
	currentPlayer := actor.GetPlayer()
	if currentPlayer == nil {
		return
	}
	towerMgr := currentPlayer.GetTowerMgr()
	if towerMgr == nil {
		return
	}
	result, ok := arg.Get("settlement")
	if !ok {
		return
	}
	settlement, ok := result.(*battle.Settlement)
	if !ok || settlement == nil {
		return
	}
	if settlement.BattleType != pb_battle.BattleType_BATTLE_TYPE_TOWER ||
		settlement.Outcome != pb_battle.BattleOutcome_BATTLE_OUTCOME_ATTACKER_WIN {
		return
	}
	uid := currentPlayer.GetUID()
	participantFound := false
	for _, participant := range settlement.Participants {
		if participant.UID == uid {
			participantFound = true
			break
		}
	}
	if !participantFound {
		tlog.Log.Error("[ctl_tower/OnTowerBattleResults] 结算不包含当前玩家, pid=%v battleID=%v", uid, settlement.BattleID)
		return
	}

	before := towerMgr.GetLayer()
	after := before + 1
	towerMgr.SetLayer(after)
	currentPlayer.MarkDirty()
	tlog.Log.Info("[ctl_tower/OnTowerBattleResults] 玩家爬塔进度已更新, pid=%v before=%v after=%v battleID=%v",
		uid, before, after, settlement.BattleID)
}

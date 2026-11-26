package internal

import (
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/tlog"
)

// towerDropEquipID 是爬塔试炼中可拾取的缚灵护腕配置编号。
const towerDropEquipID = 4

// towerDropGrantID 标识每场每个参战单位仅能领取一次的装备。
const towerDropGrantID = "tower-hp-drop"

// offerTowerDrop 在角色将敌人打到四分之三生命及以下时，为该角色登记本场掉落。
func (a *BattleActor) offerTowerDrop(events []*pb.BattleEvent) {
	b := a.p.Battle
	if a.p.Request.GetBattleType() != pb.BattleType_BATTLE_TYPE_TOWER || b == nil || b.InputDone() {
		return
	}
	for _, event := range events {
		if event.GetType() != pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			continue
		}
		actorID := event.GetActorId()
		for _, owner := range a.members {
			if !owner.owns(actorID) {
				continue
			}
			if b.ReceivedGrant(actorID, towerDropGrantID) != nil {
				continue
			}
			pending := false
			for _, offer := range a.offers[actorID] {
				if offer.GetGrantId() == towerDropGrantID {
					pending = true
					break
				}
			}
			if pending {
				continue
			}
			for _, unit := range b.UnitResults() {
				if unit.GetInstanceId() != event.GetTargetId() || unit.GetTeam() != pb.BattleTeam_BATTLE_TEAM_DEFENDER {
					continue
				}
				maxHP := unit.GetMaxHp()
				if event.GetHpAfter() > (maxHP/4)*3+(maxHP%4)*3/4 {
					continue
				}
				req := &sspb.ApplyBattleGrantReq{Uid: owner.UID, BattleId: b.ID(), UnitId: actorID, RequirePickup: true,
					Grant: &pb.BattleGrant{GrantId: towerDropGrantID, Effect: &pb.BattleGrant_EquipConfigId{EquipConfigId: towerDropEquipID}}}
				if err := a.issueGrant(owner, req); err != nil && tlog.Log != nil {
					tlog.Log.Warn("[战斗/offerTowerDrop] 登记装备掉落失败, battle=%v unit=%v pid=%v err=%v", b.ID(), actorID, owner.UID, err)
				}
			}
		}
	}
}

package asset_test

import (
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/asset"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"
)

// TestActvUsesCommonAssets 验证活动共享普通资源，失败不产生部分扣除或奖励。
func TestActvUsesCommonAssets(t *testing.T) {
	glconf.SetBattleEquipCfgForTest([]*glconf.BattleEquipCfg{{CfgID: 1, Slot: 1, InitialLevel: 1}})
	p := &player.Player{UID: 7, AssetMgr: &asset.Mgr{}, EquipmentMgr: equipment.New()}
	tx := &ipb.ActvAssetTrade{UID: 7, ActvID: 100, Sequence: 1, Rewards: []*ipb.ActvAssetReward{{Type: 1, ID: 1000301, Count: 10}}}
	if asset.ApplyAssetTrade(p, tx) != pb_common.ErrCode_OK || p.GetAssetMgr().Get(1000301) != 10 {
		t.Fatal("活动奖励未进入普通资源")
	}
	buy := &ipb.ActvAssetTrade{UID: 7, ActvID: 200, Sequence: 1, CostID: 1000301, CostCount: 5, Rewards: []*ipb.ActvAssetReward{{Type: 2, ID: 1, Count: 1}}}
	if asset.ApplyAssetTrade(p, buy) != pb_common.ErrCode_OK || p.GetAssetMgr().Get(1000301) != 5 || len(p.GetEquipmentMgr().GetItems()) != 1 {
		t.Fatal("其他活动未使用同一份资源")
	}
	buy.Sequence = 2
	buy.CostCount = 6
	if asset.ApplyAssetTrade(p, buy) == pb_common.ErrCode_OK || p.GetAssetMgr().Get(1000301) != 5 {
		t.Fatal("余额不足仍扣除")
	}
	buy.CostCount = 1
	buy.Rewards = append(buy.Rewards, &ipb.ActvAssetReward{Type: 2, ID: 999, Count: 1})
	if asset.ApplyAssetTrade(p, buy) == pb_common.ErrCode_OK || p.GetAssetMgr().Get(1000301) != 5 || len(p.GetEquipmentMgr().GetItems()) != 1 {
		t.Fatal("无效奖励产生部分变更")
	}
	snapshot := p.DeepCopy()
	snapshot.GetAssetMgr().Add(1000301, 100)
	if p.GetAssetMgr().Get(1000301) != 5 {
		t.Fatal("存盘快照共享资源")
	}
	other := &player.Player{UID: 8, AssetMgr: &asset.Mgr{}}
	if asset.ApplyAssetTrade(other, tx) == pb_common.ErrCode_OK {
		t.Fatal("跨玩家发奖")
	}
}

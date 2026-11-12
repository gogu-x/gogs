package player

import (
	"github.com/gogu-x/gogs/game/play/internal/player/asset"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/google/uuid"
	"math"
)

// ApplyAssetTrade 先检查消耗，再统一扣费和发放普通道具、装备；失败不修改资产。
func (p *Player) ApplyAssetTrade(tx *ipb.ActvAssetTrade) pb_common.ErrCode {
	if p == nil || tx == nil || tx.GetUID() != p.GetUID() || tx.GetActvID() <= 0 || tx.GetSequence() == 0 {
		return pb_common.ErrCode_PARAM
	}
	if tx.GetCostCount() < 0 || tx.GetCostID() < 0 || (tx.GetCostID() == 0) != (tx.GetCostCount() == 0) || len(tx.GetRewards()) == 0 {
		return pb_common.ErrCode_PARAM
	}
	costs := make(map[uint64]int64)
	if tx.GetCostCount() > 0 {
		costs[uint64(tx.GetCostID())] = tx.GetCostCount()
	}
	assets := p.AssetMgr
	if !assets.CheckCosts(costs) {
		return pb_common.ErrCode_PARAM
	}
	rewards := make(map[uint64]int64)
	var items []*equipment.Item
	mgr := p.GetEquipmentMgr()
	for _, r := range tx.GetRewards() {
		if r == nil || r.GetID() <= 0 || r.GetCount() <= 0 {
			return pb_common.ErrCode_PARAM
		}
		id, count := r.GetID(), r.GetCount()
		switch r.GetType() {
		case 1:
			if rewards[uint64(id)] > math.MaxInt64-count {
				return pb_common.ErrCode_PARAM
			}
			rewards[uint64(id)] += count
		case 2:
			cfg := glconf.GetBattleEquipCfgNoErr(id)
			if cfg == nil || mgr == nil || count > 10000 || int64(len(items))+count > 10000 {
				return pb_common.ErrCode_CONFIG_UNAVAILABLE
			}
			for i := int64(0); i < count; i++ {
				items = append(items, &equipment.Item{ID: uuid.NewString(), ConfigID: id, Slot: cfg.GetSlot(), Level: cfg.GetInitialLevel(), RefineLevel: cfg.GetInitialRefineLevel()})
			}
		default:
			return pb_common.ErrCode_PARAM
		}
	}
	if assets == nil {
		assets = &asset.Mgr{}
	}
	if !assets.Exchange(costs, rewards) {
		return pb_common.ErrCode_PARAM
	}
	p.AssetMgr = assets
	for _, item := range items {
		mgr.AddItem(item)
	}
	p.MarkDirty()
	return pb_common.ErrCode_OK
}

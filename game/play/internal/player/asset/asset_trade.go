package asset

import (
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/tlog"
	"math"
)

// TradePlayer 提供资产交易需要的玩家内存数据，避免资产模块依赖玩家聚合包。
type TradePlayer interface {
	GetUID() uint64
	GetAssetMgr() *Mgr
	SetAssetMgr(*Mgr)
	GetEquipmentMgr() *equipment.Mgr
	MarkDirty()
}

// ApplyAssetTrade 先检查消耗，再统一扣费和发放普通道具、装备；失败不修改资产。
func ApplyAssetTrade(p TradePlayer, tx *ipb.ActvAssetTrade) (code pb_common.ErrCode) {
	defer func() {
		if p != nil {
			tlog.Log.Info("[asset/ApplyAssetTrade] 资产交易处理完成, pid=%v, actv=%v, sequence=%v, code=%v",
				p.GetUID(), tx.GetActvID(), tx.GetSequence(), code)
		}
	}()
	if p == nil || tx == nil || tx.GetUID() != p.GetUID() || tx.GetActvID() <= 0 || tx.GetSequence() == 0 {
		return pb_common.ErrCode_PARAM
	}
	costID, costCount := tx.GetCostID(), tx.GetCostCount()
	if costCount < 0 || costID < 0 || (costID == 0) != (costCount == 0) || len(tx.GetRewards()) == 0 {
		return pb_common.ErrCode_PARAM
	}
	costs := make(map[uint64]int64)
	if costCount > 0 {
		costs[uint64(costID)] = costCount
	}
	assets := p.GetAssetMgr()
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
			if mgr == nil || count > 10000 || int64(len(items))+count > 10000 {
				return pb_common.ErrCode_CONFIG_UNAVAILABLE
			}
			prepared, code := equipment.PrepareTradeItems(id, count)
			if code != pb_common.ErrCode_OK {
				return code
			}
			items = append(items, prepared...)
		default:
			return pb_common.ErrCode_PARAM
		}
	}
	if assets == nil {
		assets = &Mgr{}
	}
	before := make(map[uint64]int64, len(costs)+len(rewards))
	for id := range costs {
		before[id] = assets.Get(id)
	}
	for id := range rewards {
		before[id] = assets.Get(id)
	}
	if !assets.Exchange(costs, rewards) {
		return pb_common.ErrCode_PARAM
	}
	p.SetAssetMgr(assets)
	for _, item := range items {
		mgr.AddItem(item)
	}
	p.MarkDirty()
	after := make(map[uint64]int64, len(before))
	for id := range before {
		after[id] = assets.Get(id)
	}
	tlog.Log.Info("[asset/ApplyAssetTrade] 资产交易已生效, before=%v, after=%v, equipmentAdded=%v, pid=%v",
		before, after, len(items), p.GetUID())
	return pb_common.ErrCode_OK
}

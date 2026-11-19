package shop

import (
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"math"
)

// handleGet 查询商店，不读取主活动的内部状态。
// @actv.request
func (b *Shop) handleGet(uid uint64, request *pb_actv.GetActvShopReq) {
	b.refresh(uid, comm.Now())
	ack := &pb_actv.GetActvShopAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE}
	if b.valid {
		ack.Data = b.state(uid)
		if ack.GetData() != nil {
			ack.Code = pb_common.ErrCode_OK
		}
	}
	b.PushToPlayer(uid, ack)
}

// handleBuy 检查活动开放、商品、数量及限购，再委托通用接口检查道具余额。
// @actv.request
func (b *Shop) handleBuy(uid uint64, request *pb_actv.BuyActvShopReq) {
	ack := &pb_actv.BuyActvShopAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE, Sequence: request.GetSequence()}
	sent := false
	defer func() {
		if !sent {
			ack.Data = b.state(uid)
			b.PushToPlayer(uid, ack)
		}
	}()
	d := b.playerData()
	if !b.valid || d == nil {
		return
	}
	now := comm.Now()
	b.refresh(uid, now)
	if b.buying {
		ack.Pending = true
		return
	}
	m := b.Meta()
	ts := now.Unix()
	if b.Actv.IsEnded() || ts < m.GetStartTs() || (m.GetEndTs() > 0 && ts >= m.GetEndTs()) {
		ack.Code = pb_common.ErrCode_ACTV_NOT_OPEN
		return
	}
	seq, count := request.GetSequence(), int64(request.GetCount())
	g := b.goods[request.GetGoodsId()]
	if seq == 0 || seq != d.GetSequence()+1 || seq == math.MaxUint64 || g == nil || count <= 0 || count > 100 {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	old := d.GetBought()[g.GetCfgID()]
	if old > math.MaxInt64-count || (g.GetLimit() > 0 && old+count > int64(g.GetLimit())) {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	if g.GetCostCount() > math.MaxInt64/count {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	rewards, ok := goodsRewards(g.GetRewardGroupID(), count)
	if !ok {
		return
	}
	tx := &ipb.ActvAssetTrade{UID: uid, ActvID: m.GetActvID(), Sequence: seq, CostID: g.GetCostItemID(), CostCount: g.GetCostCount() * count, Rewards: rewards}
	day := d.GetDayStart()
	id := g.GetCfgID()
	b.buying = true
	submitted := ipb.RequestActvAssets(tx, func(code pb_common.ErrCode) {
		b.buying = false
		ack.Code = code
		b.refresh(uid, comm.Now())
		d.Sequence = seq
		if ack.GetCode() == pb_common.ErrCode_OK {
			if g.GetLimitResetType() == 0 || day == d.GetDayStart() {
				d.GetBought()[id] += count
			}
			for _, reward := range rewards {
				ack.Rewards = append(ack.GetRewards(), &pb_actv.ActvRewardItem{RewardType: reward.GetType(), RewardId: reward.GetID(), Count: reward.GetCount()})
			}
			b.Actv.GetService().HandleEvent(gameevent.NewTask(uid, task.Event{Goal: 3, Count: count, Param: int64(id)}))
		}
		b.data.MarkDirty()
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
		tlog.Log.Info("[shop/handleBuy] 购买回调完成, pid=%v, sequence=%v, code=%v", uid, seq, ack.GetCode())
	})
	if !submitted {
		b.buying = false
		return
	}
	sent = true

}

// handleRefresh 按每日规则重置限购并返回最新商店信息。
// @actv.request
func (b *Shop) handleRefresh(uid uint64, request *pb_actv.RefreshActvShopReq) {
	ack := &pb_actv.RefreshActvShopAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE}
	if b.valid {
		b.refresh(uid, comm.Now())
		ack.Data = b.state(uid)
		if ack.GetData() != nil {
			ack.Code = pb_common.ErrCode_OK
		}
	}
	b.PushToPlayer(uid, ack)
}

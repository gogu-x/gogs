//go:generate actv-router-gen
package shop

import (
	"math"
	"sort"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
)

// Shop 独立个人商店，只依赖商品配置、个人计数和通用道具交易。
type Shop struct {
	buying            bool // 购买请求等待 play 回调，防止并发突破限购。
	base.BaseBehavior      // 活动生命周期。
	data              *base.OwnedComp[PlayerData]
	goods             map[int32]*glconf.ActvShopGoodsCfg
	valid             bool
}

// Create 绑定商店自己的配置，与任务抽奖模板没有依赖。
func (*Shop) Create(a *service.Actv, c conf.ActvConfig) service.ActvBehavior {
	b := &Shop{goods: make(map[int32]*glconf.ActvShopGoodsCfg)}
	b.Bind(a, c)
	return b
}

// OnInit 注册个人商店存档并检查商品配置。
func (b *Shop) OnInit() {
	b.data = base.NewOwnedComp[PlayerData](1, uint64(b.Meta().GetOwnerID()))
	b.AddComp(b.data)
	goods, valid := loadGoods(b.Meta().GetCfgID())
	b.goods, b.valid = goods, valid
	if !valid {
		tlog.Log.Error("[shop/OnInit] 商店配置无效, actv=%v, cfg=%v", b.Meta().GetActvID(), b.Meta().GetCfgID())
	}
}

// period 返回当地零点和下一日零点。
func period(now time.Time) (int64, int64) {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return d.Unix(), d.AddDate(0, 0, 1).Unix()
}

// refresh 仅重置每日限购，不影响当期累计限购。
func (b *Shop) refresh(uid uint64, now time.Time) {
	d := b.data.Get(uid)
	if d == nil {
		return
	}
	d.Init()
	day, _ := period(now)
	if day <= d.GetDayStart() {
		return
	}
	for id, g := range b.goods {
		if g.GetLimitResetType() == 1 {
			delete(d.GetBought(), id)
		}
	}
	d.DayStart = day
	b.data.Dirty(uid)
}

// state 返回商店自己的商品次数及序号。
func (b *Shop) state(uid uint64) *pb_actv.ActvShopState {
	d := b.data.Get(uid)
	if d == nil {
		return nil
	}
	_, next := period(time.Now())
	s := &pb_actv.ActvShopState{ActvId: uint64(b.Meta().GetActvID()), NextSequence: d.GetSequence() + 1, NextResetTime: next, Pending: b.buying}
	for id := range b.goods {
		s.Goods = append(s.Goods, &pb_actv.ActvGoodsState{GoodsId: id, Bought: d.GetBought()[id]})
	}
	sort.Slice(s.Goods, func(i, j int) bool { return s.Goods[i].GetGoodsId() < s.Goods[j].GetGoodsId() })
	return s
}

// push 商店状态自行推送给Gate。
func (b *Shop) push(uid uint64) {
	if state := b.state(uid); state != nil {
		b.PushToPlayer(uid, &pb_actv.ActvShopNtf{Data: state})
	}
}

// handleLogin 登录刷新并推送商店。
// @actv.event 2 "userLogin"
func (b *Shop) handleLogin(event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	b.refresh(uid, time.Now())
	b.push(uid)
}

// handleGet 查询商店，不读取主活动的内部状态。
// @actv.request
func (b *Shop) handleGet(uid uint64, request *pb_actv.GetActvShopReq) {
	b.refresh(uid, time.Now())
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
	d := b.data.Get(uid)
	if !b.valid || d == nil {
		return
	}
	b.refresh(uid, time.Now())
	if b.buying {
		ack.Pending = true
		return
	}
	m := b.Meta()
	now := time.Now().Unix()
	if b.Actv.IsEnded() || now < m.GetStartTs() || (m.GetEndTs() > 0 && now >= m.GetEndTs()) {
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
		b.refresh(uid, time.Now())
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
		b.data.Dirty(uid)
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
		b.refresh(uid, time.Now())
		ack.Data = b.state(uid)
		if ack.GetData() != nil {
			ack.Code = pb_common.ErrCode_OK
		}
	}
	b.PushToPlayer(uid, ack)
}

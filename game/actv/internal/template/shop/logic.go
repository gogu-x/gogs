//go:generate actv-router-gen
package shop

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
	"sort"
	"time"
)

// Shop 独立个人商店，只依赖商品配置、个人计数和通用道具交易。
type Shop struct {
	buying            bool // 购买请求等待 play 回调，防止并发突破限购。
	base.BaseBehavior      // 活动生命周期。
	data              *base.Comp[PlayerData]
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
	b.data = base.NewComp[PlayerData](1)
	b.AddComp(b.data)
	goods, valid := loadGoods(b.Meta().GetCfgID())
	b.goods, b.valid = goods, valid
	if !valid {
		tlog.Log.Error("[shop/OnInit] 商店配置无效, actv=%v, cfg=%v", b.Meta().GetActvID(), b.Meta().GetCfgID())
	}
}

// refresh 仅重置每日限购，不影响当期累计限购。
func (b *Shop) refresh(uid uint64, now time.Time) {
	d := b.playerData()
	if d == nil {
		return
	}
	d.Init()
	day, _ := comm.DayBounds(now)
	if day <= d.GetDayStart() {
		return
	}
	for id, g := range b.goods {
		if g.GetLimitResetType() == 1 {
			delete(d.GetBought(), id)
		}
	}
	d.DayStart = day
	b.data.MarkDirty()
}

// state 返回商店自己的商品次数及序号。
func (b *Shop) state(uid uint64) *pb_actv.ActvShopState {
	d := b.playerData()
	if d == nil {
		return nil
	}
	_, next := comm.DayBounds(comm.Now())
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

// playerData 返回当前活动数据，加载失败时阻止业务访问默认值。
func (b *Shop) playerData() *PlayerData {
	if b.data.Blocked() {
		return nil
	}
	return b.data.GetData()
}

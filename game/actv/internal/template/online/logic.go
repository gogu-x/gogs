//go:generate go run ../../../../../tools/activity-router-gen -dir .

// Package online 实现在线活动行为和独立状态组件。
package online

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
)

// Behavior 实现在线活动行为，并组合该活动实例自己的数据组件。
type Behavior struct {
	base.BaseBehavior                              // 通用活动元数据、配置和生命周期行为。
	round             *base.Comp[RoundData]        // 实例单条数据。
	member            *base.PlayerComp[MemberData] // 玩家列表数据，仅服务器/联盟活动使用。
}

// Create 为在线活动实例创建独立的运行时行为对象。
func (*Behavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	behavior := &Behavior{}
	behavior.Bind(actv, cfg)
	return behavior
}

// OnInit 注册在线活动的数据组件。
func (b *Behavior) OnInit() {
	b.round = base.NewComp[RoundData](compRound)
	b.AddComp(b.round)
	b.member = base.NewPlayerComp[MemberData](compMember)
	b.AddPlayerComp(b.member)
}

// OnClosed 活动结束时结算内存中的在线玩家；离线玩家上线时由框架调用 NeedSettle/Settle。
func (b *Behavior) OnClosed() {
	b.member.Range(func(uid uint64, _ *MemberData) bool {
		if b.NeedSettle(uid) {
			b.Settle(uid)
		}
		return true
	})
}

// NeedSettle 判断玩家是否需要结算：未结算且有进度。
func (b *Behavior) NeedSettle(uid uint64) bool {
	p := b.member.Get(uid)
	return p != nil && !p.Settled && p.Progress > 0
}

// Settle 结算玩家。
func (b *Behavior) Settle(uid uint64) {
	p := b.member.Get(uid)
	if p == nil {
		return
	}
	p.Settled = true
	b.member.MarkDirty(uid)
}

// handleJoinActivity 处理在线活动加入请求。
// @activity.request
func (b *Behavior) handleJoinActivity(uid uint64, request *pb_activity.JoinActivityReq) {
	// 服务器活动中按 uid 取该玩家的列表条目；玩家已上线加载，不存在时自动新建。
	p := b.member.Get(uid)
	if p == nil {
		return
	}
	p.Target = b.ActvCfg.GetTarget()
	b.member.MarkDirty(uid)
}

// handleGetProgress 处理在线活动进度查询请求。
// @activity.request
func (b *Behavior) handleGetProgress(uid uint64, request *pb_activity.GetProgressReq) {
}

// handleClaimReward 处理在线活动领奖请求。
// @activity.request
func (b *Behavior) handleClaimReward(uid uint64, request *pb_activity.ClaimRewardReq) {
}

// handleUserLogin 处理玩家上线事件。
// @activity.event 1 "onUserLogin"
func (b *Behavior) handleUserLogin(event *sspb.Event) {
}

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

// handleJoinActivity 处理在线活动加入请求。
// @activity.request
func (b *Behavior) handleJoinActivity(request *pb_activity.JoinActivityReq) {
}

// handleGetProgress 处理在线活动进度查询请求。
// @activity.request
func (b *Behavior) handleGetProgress(request *pb_activity.GetProgressReq) {
}

// handleClaimReward 处理在线活动领奖请求。
// @activity.request
func (b *Behavior) handleClaimReward(request *pb_activity.ClaimRewardReq) {
}

// handleUserLogin 处理玩家上线事件。
// @activity.event 1 "onUserLogin"
func (b *Behavior) handleUserLogin(event *sspb.Event) {
}

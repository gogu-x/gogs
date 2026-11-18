package online

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
)

// handleGetProgress 处理在线活动进度查询请求。
// @actv.request
func (b *Behavior) handleGetProgress(uid uint64, request *pb_actv.GetProgressReq) {
	// 业务实现后自行构造响应并调用 PushToPlayer。
}

// handleClaimReward 处理在线活动领奖请求。
// @actv.request
func (b *Behavior) handleClaimReward(uid uint64, request *pb_actv.ClaimRewardReq) {
	// 业务实现后自行构造响应并调用 PushToPlayer。
}

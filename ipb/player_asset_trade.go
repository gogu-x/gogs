package ipb

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
)

// RequestPlayerAssetTrade 经游戏网关请求在线玩家执行一笔活动资产交易（检查消耗、扣费、发奖），
// 结果回调在活动 Actor 中执行。玩家离线时网关立即拒绝，不唤醒离线玩家。
// 返回 false 表示未投递且不会执行回调；返回 true 后回调一定会以最终结果执行一次。
func RequestPlayerAssetTrade(tx *ActvAssetTrade, callback func(pb_common.ErrCode)) bool {
	if tx == nil || callback == nil {
		return false
	}
	gatePID, ok := tree.Lookup(def.GameGate)
	if !ok {
		return false
	}
	actvPID, ok := tree.Lookup(def.ActorActv)
	if !ok {
		return false
	}
	delivery := &DeliverToPlayer{UID: tx.GetUID(), Payload: tx.DeepCopy(), RequireOnline: true}
	return tree.RequestCallback(
		gatePID,
		delivery,
		actvPID,
		func(_ tree.Context, value interface{}, err error) {
			code := pb_common.ErrCode_ACTV_UNAVAILABLE
			if result, ok := value.(pb_common.ErrCode); err == nil && ok {
				code = result
			}

			callback(code)
		},
	) != nil
}

package ipb

import (
	"fmt"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
)

// RequestActvAssets 请求 play 检查资产、扣费并发奖，结果回调在活动 Actor 中执行。
// 返回 false 表示未投递且不会执行回调；返回 true 后由回调报告最终结果。
func RequestActvAssets(tx *ActvAssetTrade, callback func(pb_common.ErrCode)) bool {
	if tx == nil || callback == nil {
		return false
	}
	sys := tree.Default()
	if sys == nil {
		return false
	}
	target, ok := sys.Lookup(fmt.Sprintf("player-%d", tx.GetUID()))
	if !ok {
		return false
	}
	sender, ok := sys.Lookup(def.ActorActv)
	if !ok {
		return false
	}
	snapshot := tx.DeepCopy()
	return sys.RequestCallback(
		target,
		snapshot,
		sender,
		func(_ tree.Context, value interface{}, err error) {
			code := pb_common.ErrCode_ACTV_UNAVAILABLE
			if result, ok := value.(pb_common.ErrCode); err == nil && ok {
				code = result
			}

			callback(code)
		},
	) != nil
}

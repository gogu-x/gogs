//go:generate deepcopy-gen
package tasklottery

import (
	"github.com/gogu-x/gogs/ipb"
)

// Reward 是通用奖励快照，任务和抽奖仅负责选择奖励。
type Reward = ipb.ActvAssetReward

// PlayerData 保存本活动日期与请求状态，任务进度由TaskComp独立保存，不保存商店限购或玩家道具余额。
// +deepcopy-gen=true
//
//go:generate accessor-gen
type PlayerData struct {
	DayStart int64  `bson:"day_start"` // 最近刷新日期。
	LoginDay int64  `bson:"login_day"` // 最近计入的登录日期。
	Sequence uint64 `bson:"sequence"`  // 最近已受理写请求序号。
}

// Init 保留空数据初始化接口，任务组件负责初始化任务容器。
func (*PlayerData) Init() {}

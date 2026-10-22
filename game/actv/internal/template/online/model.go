package online

const (
	// compRound 标识在线活动实例的单条数据组件。
	compRound int32 = 1
	// compMember 标识在线活动的玩家列表组件。
	compMember int32 = 2
)

// RoundData 保存在线活动实例自己的状态。
type RoundData struct {
	Round int32 `bson:"round"` // 当前轮次。
}

// MemberData 保存在线活动中单个玩家的完成状态。
type MemberData struct {
	Progress int32 `bson:"progress"`           // 当前活动进度。
	Target   int32 `bson:"target"`             // 活动完成目标。
	Rewarded bool  `bson:"rewarded,omitempty"` // 是否已领取奖励。
}

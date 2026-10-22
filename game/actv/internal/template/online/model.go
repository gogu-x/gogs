package online

const (
	// compRound 标识在线活动实例的单条数据组件。
	compRound int32 = 1
	// compMember 标识在线活动的玩家列表组件。
	compMember int32 = 2
)

// RoundData 保存在线活动实例自己的状态。
type RoundData struct {
	Round int32            `bson:"round"`           // 当前轮次。
	Ranks map[string]int32 `bson:"ranks,omitempty"` // 玩家名次（键为 uid 字符串），活动结束时固定。
}

// Init 补齐运行时容器；新建和加载后都会调用，只处理为空的字段。
func (d *RoundData) Init() {
	if d.Ranks == nil {
		d.Ranks = make(map[string]int32)
	}
}

// MemberData 保存在线活动中单个玩家的完成状态。
type MemberData struct {
	Progress int32           `bson:"progress"`           // 当前活动进度。
	Target   int32           `bson:"target"`             // 活动完成目标。
	Rewarded bool            `bson:"rewarded,omitempty"` // 是否已领取奖励。
	Settled  bool            `bson:"settled,omitempty"`  // 活动结束后是否已结算，由本模板自行维护。
	Tasks    map[int32]int32 `bson:"tasks,omitempty"`    // 子任务进度。
}

// Init 补齐运行时容器；新建和加载后都会调用，只处理为空的字段。
func (d *MemberData) Init() {
	if d.Tasks == nil {
		d.Tasks = make(map[int32]int32)
	}
}

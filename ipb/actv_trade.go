//go:generate deepcopy-gen
package ipb

// ActvAssetReward 是通用道具交易中的确定奖励快照。
// +deepcopy-gen=true
//
//go:generate accessor-gen
type ActvAssetReward struct {
	Type  int32 `bson:"type"`  // 1普通资源，2装备。
	ID    int32 `bson:"id"`    // 配置编号。
	Count int64 `bson:"count"` // 数量。
}

// ActvAssetTrade 由活动投递给玩家统一扣除和发奖。
// +deepcopy-gen=true
//
//go:generate accessor-gen
type ActvAssetTrade struct {
	UID       uint64             `bson:"uid"`        // 玩家编号。
	ActvID    int64              `bson:"actv_id"`    // 发起交易的活动实例。
	Sequence  uint64             `bson:"sequence"`   // 发起活动内单调递增的玩家交易序号。
	CostID    int32              `bson:"cost_id"`    // 消耗道具，0表示无消耗。
	CostCount int64              `bson:"cost_count"` // 消耗数量。
	Rewards   []*ActvAssetReward `bson:"rewards"`    // 奖励快照。
}

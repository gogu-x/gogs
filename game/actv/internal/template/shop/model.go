//go:generate deepcopy-gen
package shop

// PlayerData 保存商店自己的限购与交易状态。
// +deepcopy-gen=true
//
//go:generate accessor-gen
type PlayerData struct {
	Bought   map[int32]int64 `bson:"bought"`    // 商品限购周期内已购份数。
	DayStart int64           `bson:"day_start"` // 最近每日重置日期。
	Sequence uint64          `bson:"sequence"`  // 商店独立交易序号。
}

// Init 初始化商店计数容器。
func (d *PlayerData) Init() {
	if d.GetBought() == nil {
		d.Bought = make(map[int32]int64)
	}
}

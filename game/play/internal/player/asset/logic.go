package asset

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"math"
	"sort"
)

// CheckCosts 检查全部消耗是否有效且余额充足，不修改资产。
func (m *Mgr) CheckCosts(costs map[uint64]int64) bool {
	for id, count := range costs {
		if id == 0 || count <= 0 || m == nil || m.Get(id) < count {
			return false
		}
	}
	return true
}

// Exchange 先检查全部消耗和奖励，再统一修改普通资源；失败时不产生部分变化。
func (m *Mgr) Exchange(costs, rewards map[uint64]int64) bool {
	if m == nil || !m.CheckCosts(costs) {
		return false
	}
	next := make(map[uint64]int64, len(costs)+len(rewards))
	for id, count := range costs {
		next[id] = m.Get(id) - count
	}
	for id, count := range rewards {
		if id == 0 || count <= 0 {
			return false
		}
		balance, ok := next[id]
		if !ok {
			balance = m.Get(id)
		}
		if balance < 0 || balance > math.MaxInt64-count {
			return false
		}
		next[id] = balance + count
	}
	if m.Assets == nil {
		m.Assets = make(map[uint64]int64)
	}
	for id, balance := range next {
		m.Assets[id] = balance
	}
	return true
}

// Message 打包普通资源余额，所有业务统一使用此推送。
func (m *Mgr) Message() *pb_common.AssetsNtf {
	message := &pb_common.AssetsNtf{}
	if m == nil {
		return message
	}
	for id, count := range m.Assets {
		message.Assets = append(message.Assets, &pb_common.AssetBalance{Id: id, Count: count})
	}
	sort.Slice(message.Assets, func(i, j int) bool { return message.Assets[i].GetId() < message.Assets[j].GetId() })
	return message
}

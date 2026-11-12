package asset

import (
	"math"
	"testing"
)

// TestExchange 验证统一资源兑换的原子校验、同道具收支与溢出处理。
func TestExchange(t *testing.T) {
	m := &Mgr{Assets: map[uint64]int64{1: 10, 2: math.MaxInt64}}
	if m.Exchange(map[uint64]int64{1: 11}, map[uint64]int64{3: 1}) || m.Get(1) != 10 || m.Get(3) != 0 {
		t.Fatal("不能用奖励抵消不足消耗")
	}
	if m.Exchange(map[uint64]int64{1: 1}, map[uint64]int64{2: 1}) || m.Get(1) != 10 {
		t.Fatal("溢出时产生扣除")
	}
	if !m.Exchange(map[uint64]int64{1: 5}, map[uint64]int64{1: 2, 3: 4}) || m.Get(1) != 7 || m.Get(3) != 4 {
		t.Fatal("普通资源收支错误")
	}
	if m.Exchange(map[uint64]int64{1: -1}, nil) || m.Get(1) != 7 {
		t.Fatal("负数消耗改变资源")
	}
}

// TestCheckCosts 验证余额预检不会修改资产，且拒绝非法消耗。
func TestCheckCosts(t *testing.T) {
	m := &Mgr{Assets: map[uint64]int64{1: 10}}
	for _, count := range []int64{-1, 0, 11} {
		if m.CheckCosts(map[uint64]int64{1: count}) {
			t.Fatalf("错误接受消耗 %d", count)
		}
	}
	if !m.CheckCosts(map[uint64]int64{1: 10}) || m.Get(1) != 10 {
		t.Fatal("预检修改余额或错误拒绝足额消耗")
	}
	var empty *Mgr
	if empty.CheckCosts(map[uint64]int64{1: 1}) || !empty.CheckCosts(nil) {
		t.Fatal("空资产检查错误")
	}
}

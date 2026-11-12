package trigger

import (
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/tree/tlog"
	"testing"
	"time"
)

// TestUnifiedCreationRules 验证同一引擎按归属、配置和创建历史分流，支持注册扩展规则。
func TestUnifiedCreationRules(t *testing.T) {
	tlog.NewLog("", 0)
	rows := []*glconf.ActvOnlineCfg{
		{ID: 1, Type: 7001, Available: true},
		{ID: 2, Type: 7001, Available: false},
		{ID: 3, Type: 7001, Available: true, Reopen: true},
		{ID: 4, Type: 9999, Available: true},
		{ID: 5, Type: 7001, Available: true, ServerGroups: []int32{1}},
		{ID: 6, Type: 1, Available: true, TriggerType: 1, TriggerVal: "2026-11-12 00:00:00", Duration: "1d"},
	}
	scopeOf := func(typ int32) int32 {
		if typ == 7001 {
			return 1
		}
		if typ == 1 {
			return 2
		}
		return 0
	}
	engine := NewEngine(
		rows, scopeOf, 0, time.UTC,
	)
	now := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC).Unix()
	result := engine.Check(OnPlayerInit, now, nil)
	if len(result.PlayerConfigs()) != 1 || result.PlayerConfigs()[0].GetID() != 1 {
		t.Fatal("个人配置筛选错误")
	}
	if len(engine.Check(OnPlayerInit, now, map[int32]bool{1: true}).PlayerConfigs()) != 0 {
		t.Fatal("历史活动重复创建")
	}
	if len(engine.Check(OnServerTick, now, nil).ServerPlans()) != 1 {
		t.Fatal("服务器时间计划未触发")
	}
	if len(engine.Check(OnServerTick, now-86400, nil).ServerPlans()) != 0 {
		t.Fatal("服务器活动提前触发")
	}
	const custom Point = 99 // 测试自定义入口。
	engine.Register(custom, func(_ int64, _ map[int32]bool) Selection { return NewSelection(nil, rows[:1]) })
	if len(engine.Check(custom, now, nil).PlayerConfigs()) != 1 {
		t.Fatal("扩展规则未生效")
	}
}

package engine

import (
	"testing"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func TestTimedStatusRefreshAndRemoval(t *testing.T) {
	setup := testSetup()
	setup.Rules.TickDurationMS = 100
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	source, target := b.units[0], b.units[1]
	config := StatusConfig{ID: "slow", DurationMS: 300}
	if err := b.applyStatus(source, target, "skill", config); err != nil {
		t.Fatal(err)
	}
	id := target.Statuses[0].InstanceID
	b.tick = 1
	if err := b.applyStatus(source, target, "skill", config); err != nil {
		t.Fatal(err)
	}
	if len(target.Statuses) != 1 || target.Statuses[0].InstanceID != id || target.Statuses[0].Version != 2 {
		t.Fatalf("刷新应复用实例并增加版本: %+v", target.Statuses)
	}
	b.Advance()
	if b.tick != 3 || len(target.Statuses) != 1 {
		t.Fatal("旧到期任务误删刷新后的状态")
	}
	b.Advance()
	if b.tick != 4 || len(target.Statuses) != 0 {
		t.Fatal("刷新后的状态没有按时移除")
	}
	removed := 0
	for _, event := range b.events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED {
			removed++
			if event.GetStatusVersion() != 2 || event.GetStatusInstanceId() != id {
				t.Fatal("移除元数据错误")
			}
		}
	}
	if removed != 1 {
		t.Fatalf("移除次数=%d", removed)
	}
}

func TestTimedStatusSeparateSourcesAndEarlyRemoval(t *testing.T) {
	setup := testSetup()
	setup.Rules.TickDurationMS = 100
	b, err := NewBattle(setup)
	if err != nil {
		t.Fatal(err)
	}
	target := b.units[1]
	config := StatusConfig{ID: "slow", DurationMS: 201}
	for _, source := range b.units {
		if err := b.applyStatus(source, target, "skill", config); err != nil {
			t.Fatal(err)
		}
	}
	if len(target.Statuses) != 2 || target.Statuses[0].InstanceID == target.Statuses[1].InstanceID {
		t.Fatal("不同来源应为独立实例")
	}
	if target.Statuses[0].ExpireTick != 3 {
		t.Fatal("毫秒应向上取整到逻辑帧")
	}
	b.removeStatus(target, target.Statuses[0].InstanceID)
	b.Advance()
	if len(target.Statuses) != 0 {
		t.Fatal("剩余实例未到期")
	}
	removed := 0
	for _, event := range b.events {
		if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED {
			removed++
		}
	}
	if removed != 2 {
		t.Fatalf("提前移除后不应重复移除，次数=%d", removed)
	}
}

func TestStatusDurationValidation(t *testing.T) {
	invalid := []StatusConfig{
		{}, {DurationMS: -1}, {DurationTurns: -1}, {DurationMS: 100, DurationTurns: 1},
		{DurationTurns: 1, IntervalMS: 100}, {DurationMS: 100, IntervalMS: -1},
		{DurationMS: 100, Kind: pb.StatusType_STATUS_TYPE_DOT},
		{DurationMS: 100, Kind: pb.StatusType_STATUS_TYPE_HOT},
		{DurationMS: 100, IntervalMS: 10},
	}
	for _, config := range invalid {
		if config.Validate() == nil {
			t.Fatalf("应拒绝配置: %+v", config)
		}
	}
	for _, config := range []StatusConfig{{DurationTurns: 1}, {DurationMS: 100}, {DurationMS: 100, IntervalMS: 10, Kind: pb.StatusType_STATUS_TYPE_DOT}} {
		if err := config.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

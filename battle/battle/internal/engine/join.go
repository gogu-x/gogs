package engine

import (
	"fmt"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// AddAttacker 在当前战斗时间加入一个单位。所有初始状态成功创建后才合入本场时间轴。
func (b *Battle) AddAttacker(config UnitConfig, started bool) error {
	if b.done || b.finished || config.Team != pb.BattleTeam_BATTLE_TEAM_ATTACKER {
		return fmt.Errorf("当前战斗无法入场")
	}
	for _, existing := range b.units {
		if existing.Config.Team == config.Team && existing.Config.Position >= config.Position {
			config.Position = existing.Config.Position + 1
		}
	}
	setup := b.setup
	setup.Units = append(setup.Units[:len(setup.Units):len(setup.Units)], config)
	if err := setup.validate(); err != nil {
		return err
	}
	u := &unit{Config: config, HP: config.MaxHP, Cooldowns: make(map[string]int), charges: make(map[pb.EffectType]int64)}
	staged := &Battle{setup: setup, strategies: b.strategies, elapsedMS: b.elapsedMS, nextStatus: b.nextStatus}
	if started {
		event := staged.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_UNIT_JOINED, config.InstanceID, "", "", "", 0, 0, u.HP, eventInfo{}, nil)
		event.JoinedUnit = u.snapshot()
	}
	for _, status := range config.InitialStatus {
		if err := staged.applyStatus(u, u, "", status); err != nil {
			return err
		}
	}
	b.setup.Units = setup.Units
	b.units = append(b.units, u)
	b.inputs[config.InstanceID] = &inputState{actor: u}
	b.nextStatus = staged.nextStatus
	for _, item := range staged.timeline {
		b.schedule(item)
	}
	for _, event := range staged.events {
		event.Sequence = uint64(len(b.events) + 1)
		b.events = append(b.events, event)
	}
	return nil
}

// SnapshotCursor 标记当前血量快照对应的事件边界。
func (b *Battle) SnapshotCursor() (uint64, int64) { return uint64(len(b.events)), b.elapsedMS }

// StatusSnapshot 导出在场状态的独立快照，新加入者可从快照序号继续接收后续事件。
func (b *Battle) StatusSnapshot() []*pb.BattleStatusView {
	var result []*pb.BattleStatusView
	for _, u := range b.units {
		for _, s := range u.Statuses {
			result = append(result, &pb.BattleStatusView{UnitId: u.Config.InstanceID, StatusId: s.Config.ID, InstanceId: s.InstanceID, Version: s.Version, ExpireMs: s.ExpireMs, RemainingTurns: int32(s.Remaining), SourceId: s.SourceID})
		}
	}
	return result
}

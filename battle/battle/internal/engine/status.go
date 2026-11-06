package engine

import (
	"fmt"
	"math"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Validate 检查持续时间与周期配置，避免同时使用两种计时方式。
func (s StatusConfig) Validate() error {
	if s.DurationMS < 0 || s.IntervalMS < 0 || s.DurationTurns < 0 {
		return fmt.Errorf("状态 %s 的持续时间和间隔不能为负数", s.ID)
	}
	if s.DurationMS == 0 {
		if s.DurationTurns == 0 || s.IntervalMS != 0 {
			return fmt.Errorf("状态 %s 的回合模式需要正数回合且不能配置毫秒间隔", s.ID)
		}
		return nil
	}
	if s.DurationTurns != 0 {
		return fmt.Errorf("状态 %s 不能同时配置持续回合和毫秒数", s.ID)
	}
	periodic := s.Kind == pb.StatusType_STATUS_TYPE_DOT || s.Kind == pb.StatusType_STATUS_TYPE_HOT
	if periodic && s.IntervalMS == 0 {
		return fmt.Errorf("限时持续伤害或治疗状态 %s 需要配置生效间隔", s.ID)
	}
	if !periodic && s.IntervalMS != 0 {
		return fmt.Errorf("非周期状态 %s 不能配置生效间隔", s.ID)
	}
	return nil
}

// IsActive 判断状态是否仍在生效，限时状态由时间轴负责移除。
func (s StatusInstance) IsActive() bool {
	return s.Config.DurationMS > 0 || s.Remaining > 0
}

// durationTicks 向上取整为逻辑帧，使用除法和余数避免加法溢出。
func (b *Battle) durationTicks(milliseconds int64) int64 {
	tickMS := int64(b.setup.Rules.TickDurationMS)
	ticks := milliseconds / tickMS
	if milliseconds%tickMS != 0 {
		ticks++
	}
	return ticks
}

// statusIndex 定位本场唯一状态实例。
func (u *unit) statusIndex(id uint64) int {
	for index := range u.Statuses {
		if u.Statuses[index].InstanceID == id {
			return index
		}
	}
	return -1
}

// applyStatus 统一处理初始状态和技能状态，同来源限时状态刷新，其他状态独立加入。
func (b *Battle) applyStatus(
	source, target *unit, skillID string, config StatusConfig,
) error {
	if err := config.Validate(); err != nil {
		return err
	}
	instance := StatusInstance{Config: config, Remaining: config.DurationTurns, SourceID: source.Config.InstanceID, SkillID: skillID, Version: 1}
	if config.DurationMS > 0 {
		ticks := b.durationTicks(config.DurationMS)
		if b.tick > math.MaxInt64-ticks {
			return fmt.Errorf("状态 %s 到期逻辑帧溢出", config.ID)
		}
		instance.ExpireTick = b.tick + ticks
		for index, previous := range target.Statuses {
			if previous.Config.DurationMS > 0 && previous.Config.ID == config.ID && previous.SourceID == instance.SourceID {
				instance.InstanceID = previous.InstanceID
				instance.Version = previous.Version + 1
				target.Statuses[index] = instance
				b.scheduleStatus(target, instance)
				b.emitStatus(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, target, instance)
				return nil
			}
		}
	}
	statuses, err := b.strategies.Statuses.Apply(target.Statuses, config)
	if err != nil {
		return err
	}
	target.Statuses = statuses
	for index, status := range statuses {
		if status.InstanceID != 0 {
			continue
		}
		b.nextStatus++
		instance.InstanceID = b.nextStatus
		// 保留状态策略对配置与剩余回合的处理结果。
		instance.Config, instance.Remaining = status.Config, status.Remaining
		target.Statuses[index] = instance
		b.scheduleStatus(target, instance)
		b.emitStatus(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, target, instance)
	}
	return nil
}

// scheduleStatus 安排到期和首次周期任务，刷新版本使历史任务自动失效。
func (b *Battle) scheduleStatus(target *unit, status StatusInstance) {
	if status.Config.DurationMS == 0 {
		return
	}
	b.schedule(&timelineItem{Tick: status.ExpireTick, Phase: phaseStatusExpire, Actor: target, StatusID: status.InstanceID, StatusVersion: status.Version})
	b.scheduleStatusTick(target, status)
}

// scheduleStatusTick 只安排严格早于到期帧的周期效果，到期帧不再跳一次。
func (b *Battle) scheduleStatusTick(target *unit, status StatusInstance) {
	interval := status.Config.IntervalMS
	if interval <= 0 {
		return
	}
	ticks := b.durationTicks(interval)
	if ticks >= status.ExpireTick-b.tick {
		return
	}
	b.schedule(&timelineItem{Tick: b.tick + ticks, Phase: phaseStatusTick, Actor: target, StatusID: status.InstanceID, StatusVersion: status.Version})
}

// handleStatusTask 忽略已移除或被刷新的任务，不让旧任务误删新状态。
func (b *Battle) handleStatusTask(item *timelineItem) {
	target := item.Actor
	index := target.statusIndex(item.StatusID)
	if index < 0 {
		return
	}
	status := target.Statuses[index]
	if status.Version != item.StatusVersion {
		return
	}
	b.action = 0
	if item.Phase == phaseStatusExpire {
		if status.ExpireTick == b.tick {
			b.removeStatus(target, status.InstanceID)
		}
		return
	}
	if target.alive() && b.tick < status.ExpireTick {
		b.applyPeriodicStatus(target, status)
		if target.alive() {
			b.scheduleStatusTick(target, status)
		}
	}
}

// removeStatus 按实例移除状态，供到期及后续驱散效果复用。
func (b *Battle) removeStatus(target *unit, instanceID uint64) bool {
	index := target.statusIndex(instanceID)
	if index < 0 {
		return false
	}
	status := target.Statuses[index]
	copy(target.Statuses[index:], target.Statuses[index+1:])
	last := len(target.Statuses) - 1
	target.Statuses[last] = StatusInstance{}
	target.Statuses = target.Statuses[:last]
	b.emitStatus(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED, target, status)
	return true
}

// emitStatus 记录施加、刷新或移除事件，客户端通过实例编号区分同名状态。
func (b *Battle) emitStatus(kind pb.BattleEventType, target *unit, status StatusInstance) {
	event := b.emit(
		kind, status.SourceID, target.Config.InstanceID, status.SkillID, status.Config.ID,
		0, target.HP, target.HP,
		eventInfo{EffectType: pb.EffectType_EFFECT_TYPE_APPLY_STATUS, StatusType: status.Config.Kind}, nil,
	)
	setStatusEvent(event, status)
}

// setStatusEvent 附加状态实例、到期时间和版本，不影响原有伤害字段。
func setStatusEvent(event *pb.BattleEvent, status StatusInstance) {
	event.StatusInstanceId = status.InstanceID
	event.StatusExpireTick = status.ExpireTick
	event.StatusVersion = status.Version
}

// applyPeriodicStatus 按状态策略执行一次周期伤害或治疗。
func (b *Battle) applyPeriodicStatus(target *unit, status StatusInstance) {
	result := b.strategies.Statuses.Tick(status)
	if result.Damage > 0 {
		before := target.HP
		amount := min(result.Damage, before)
		target.HP -= amount
		event := b.emit(
			pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE,
			status.SourceID, target.Config.InstanceID, status.SkillID, status.Config.ID,
			amount, before, target.HP, eventInfo{StatusType: status.Config.Kind}, nil,
		)
		setStatusEvent(event, status)
	}
	if result.Heal > 0 && target.alive() {
		before := target.HP
		amount := min(result.Heal, target.Config.MaxHP-before)
		target.HP += amount
		event := b.emit(
			pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL,
			status.SourceID, target.Config.InstanceID, status.SkillID, status.Config.ID,
			amount, before, target.HP, eventInfo{StatusType: status.Config.Kind}, nil,
		)
		setStatusEvent(event, status)
	}
}

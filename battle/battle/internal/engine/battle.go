package engine

import (
	"container/heap"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type activeStatus = StatusInstance

type unit struct {
	windup       *pb.BattleWindup  // 本单位已锁定目标但尚未落刀的重击。
	guardUntil   int64             // 架势有效期末端，命中时刻严格小于此值才可格挡。
	guardReady   int64             // 架势自身冷却的到期时间。
	counterReady bool              // 已经格挡成功，等待下一次有效普攻。
	grants       []*pb.BattleGrant // 按入账顺序保存，供去重及快照使用。
	Config       UnitConfig
	HP           int64
	Cooldowns    map[string]int
	Statuses     []activeStatus
	charges      map[pb.EffectType]int64
}

// alive 根据当前生命值判断单位是否存活。
func (u *unit) alive() bool { return u.HP > 0 }

// getCharge 读取指定效果的积攒值。
func (u *unit) getCharge(kind pb.EffectType) int64 { return u.charges[kind] }

// setCharge 更新指定效果的积攒值。
func (u *unit) setCharge(kind pb.EffectType, value int64) { u.charges[kind] = value }

// resetCharge 清空指定效果的积攒值。
func (u *unit) resetCharge(kind pb.EffectType) { u.charges[kind] = 0 }

// Battle 是不依赖外部服务的确定性战斗实例。
// 全部可变状态都在本结构体上，因此可以按输入及到期状态推进而不丢状态。
type Battle struct {
	inputs         map[string]*inputState
	setup          Setup
	rng            *RNG
	strategies     Strategies
	units          []*unit
	timeline       timelineQueue
	elapsedMS      int64
	action         uint64
	nextStatus     uint64 // 状态实例编号，整场战斗内单调递增。
	startedActions int

	events       []*pb.BattleEvent
	emitted      int              // events 中已返回给调用方的游标
	finalOutcome pb.BattleOutcome // 终局判定结果（outcome() 方法已占用该名字）

	done     bool // 终止条件已满足，不再推进
	finished bool
	cached   Result
}

// defaultDurationMS 为旧配置未指定时长的战斗提供三分钟上限。
const defaultDurationMS int64 = 180000

// NewBattle 使用 BattleActor 传入的运行时数据创建战斗。
func NewBattle(setup Setup) (*Battle, error) {
	if setup.Rules.MaxDurationMS == 0 {
		setup.Rules.MaxDurationMS = defaultDurationMS
	}
	if err := setup.validate(); err != nil {
		return nil, err
	}
	if err := setup.Strategies.validate(); err != nil {
		return nil, err
	}
	battle := &Battle{setup: setup, rng: NewRNG(setup.Seed), strategies: setup.Strategies}
	battle.inputs = make(map[string]*inputState)
	for _, config := range setup.Units {
		battle.units = append(battle.units, &unit{Config: config, HP: config.MaxHP, Cooldowns: make(map[string]int), charges: make(map[pb.EffectType]int64)})
	}
	for _, actor := range battle.units {
		for _, status := range actor.Config.InitialStatus {
			if err := battle.applyStatus(actor, actor, "", status); err != nil {
				return nil, err
			}
		}
		battle.inputs[actor.Config.InstanceID] = &inputState{actor: actor}
	}
	heap.Init(&battle.timeline)
	return battle, nil
}

// ID 返回 Manager 生成的战斗 ID。
func (b *Battle) ID() string { return b.setup.BattleID }

// eventInfo 承载战斗事件的结构化效果、状态和原因信息。
type eventInfo struct {
	EffectType  pb.EffectType
	StatusType  pb.StatusType
	Critical    bool
	BasicAttack bool
	Reason      pb.BattleEventReason
	Outcome     pb.BattleOutcome
}

// emit 记录本场战斗产生的事件。
func (b *Battle) emit(
	kind pb.BattleEventType, actor, target, skill, status string,
	amount, before, after int64, info eventInfo, targetIDs []string,
) *pb.BattleEvent {
	event := &pb.BattleEvent{
		Sequence: uint64(len(b.events) + 1), Action: b.action, ElapsedMs: b.elapsedMS, Type: kind,
		ActorId: actor, TargetId: target, SkillId: skill, StatusId: status,
		Amount: amount, HpBefore: before, HpAfter: after,
		EffectType: info.EffectType, StatusType: info.StatusType, Critical: info.Critical,
		BasicAttack: info.BasicAttack, Reason: info.Reason, Outcome: info.Outcome,
		TargetIds: append([]string(nil), targetIDs...),
	}
	b.events = append(b.events, event)
	return event
}

// teamAlive 判断指定阵营是否还有存活单位。
func (b *Battle) teamAlive(team pb.BattleTeam) bool {
	for _, unit := range b.units {
		if unit.Config.Team == team && unit.alive() {
			return true
		}
	}
	return false
}

// outcome 根据双方存活情况判断战斗是否结束及胜负。
func (b *Battle) outcome() (pb.BattleOutcome, bool) {
	attackerAlive := b.teamAlive(pb.BattleTeam_BATTLE_TEAM_ATTACKER)
	defenderAlive := b.teamAlive(pb.BattleTeam_BATTLE_TEAM_DEFENDER)
	switch {
	case attackerAlive && defenderAlive:
		return pb.BattleOutcome_BATTLE_OUTCOME_UNSPECIFIED, false
	case attackerAlive:
		return pb.BattleOutcome_BATTLE_OUTCOME_ATTACKER_WIN, true
	case defenderAlive:
		return pb.BattleOutcome_BATTLE_OUTCOME_DEFENDER_WIN, true
	default:
		return pb.BattleOutcome_BATTLE_OUTCOME_DRAW, true
	}
}

// attackInterval 根据单位速度计算两次攻击之间的间隔。
func (b *Battle) attackInterval(actor *unit) int64 {
	speed := b.unitSpeed(actor)
	return max(1, 1+(b.setup.Rules.ATBThreshold-1)/speed)
}

// unitView 构造策略读取的单位状态视图。
func (b *Battle) unitView(u *unit) UnitView {
	return UnitView{ID: u.Config.InstanceID, Team: u.Config.Team, Position: u.Config.Position, HP: u.HP, MaxHP: u.Config.MaxHP,
		Attack: b.unitAttack(u), Defense: b.unitDefense(u), Alive: u.alive()}
}

// unitAttack 计算单位当前的攻击力。
func (b *Battle) unitAttack(u *unit) int64 {
	return max(0, u.Config.Attack+b.unitModifier(u).AttackFlat)
}

// unitDefense 计算单位当前的防御力。
func (b *Battle) unitDefense(u *unit) int64 {
	return max(0, u.Config.Defense+b.unitModifier(u).DefenseFlat)
}

// unitSpeed 计算单位当前的速度。
func (b *Battle) unitSpeed(u *unit) int64 { return max(1, u.Config.Speed+b.unitModifier(u).SpeedFlat) }

// unitModifier 获取单位状态带来的属性修正。
func (b *Battle) unitModifier(u *unit) AttributeModifier {
	return b.strategies.Statuses.Modifier(u.Statuses)
}

// hasStatus 检查单位是否持有指定状态。
func (b *Battle) hasStatus(u *unit, kind pb.StatusType) bool {
	return b.strategies.Statuses.Has(u.Statuses, kind)
}

// tickCooldowns 推进单位的技能冷却。
func (b *Battle) tickCooldowns(actor *unit) {
	for id, turns := range actor.Cooldowns {
		if turns <= 1 {
			delete(actor.Cooldowns, id)
		} else {
			actor.Cooldowns[id] = turns - 1
		}
	}
}

// tickPeriodic 结算单位的周期状态效果。
func (b *Battle) tickPeriodic(actor *unit) {
	for _, status := range actor.Statuses {
		if !actor.alive() {
			return
		}
		if status.Config.DurationMS == 0 && status.IsActive() {
			b.applyPeriodicStatus(actor, status)
		}
	}
}

// expireStatuses 更新并清理到期状态。
func (b *Battle) expireStatuses(actor *unit) {
	if len(actor.Statuses) == 0 {
		return
	}
	old := b.attackInterval(actor)
	previous := append([]StatusInstance(nil), actor.Statuses...)
	actor.Statuses = b.strategies.Statuses.Expire(actor.Statuses)
	for _, status := range previous {
		if actor.statusIndex(status.InstanceID) < 0 {
			b.emitStatus(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_REMOVED, actor, status)
		}
	}
	b.syncStatusSpeed(actor, old)
}

// targets 按技能目标规则选取目标单位。
func (b *Battle) targets(actor *unit, rule pb.TargetRule) []*unit {
	views := make([]UnitView, 0, len(b.units))
	byID := make(map[string]*unit, len(b.units))
	for _, candidate := range b.units {
		views = append(views, b.unitView(candidate))
		byID[candidate.Config.InstanceID] = candidate
	}
	ids := b.strategies.Targets.Select(TargetQuery{Actor: b.unitView(actor), Units: views, Rule: rule})
	selected := make([]*unit, 0, len(ids))
	for _, id := range ids {
		if candidate := byID[id]; candidate != nil && candidate.alive() {
			selected = append(selected, candidate)
		}
	}
	return selected
}

// targetIDs 提取目标单位的编号。
func targetIDs(targets []*unit) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.Config.InstanceID)
	}
	return ids
}

// firstTarget 取出首个目标供动作事件使用。
func firstTarget(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// impactAction 在命中时校验参战单位并结算技能效果。
func (b *Battle) impactAction(action *battleAction) {
	actor := action.Actor
	if !actor.alive() {
		b.emit(
			pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
			actor.Config.InstanceID, firstTarget(action.TargetIDs), action.Skill.ID, "",
			0, actor.HP, actor.HP,
			eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_ACTOR_DEAD}, action.TargetIDs,
		)
		return
	}
	if len(action.Targets) == 0 {
		b.emit(
			pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
			actor.Config.InstanceID, "", action.Skill.ID, "",
			0, actor.HP, actor.HP,
			eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_TARGET_DEAD}, action.TargetIDs,
		)
		return
	}
	counter := action.Basic && actor.counterReady
	successfulHits := 0
	for _, target := range action.Targets {
		if !target.alive() {
			b.emit(
				pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
				actor.Config.InstanceID, target.Config.InstanceID, action.Skill.ID, "",
				0, target.HP, target.HP,
				eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_TARGET_DEAD}, []string{target.Config.InstanceID},
			)
			continue
		}
		results, err := b.strategies.Skills.Execute(SkillExecutionContext{
			Actor: b.unitView(actor), Target: b.unitView(target), Skill: action.Skill,
			Rules: b.setup.Rules, Effects: b.strategies.Effects, Damage: b.strategies.Damage, RNG: b.rng,
		})
		if err != nil {
			b.emit(
				pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
				actor.Config.InstanceID, target.Config.InstanceID, action.Skill.ID, "",
				0, target.HP, target.HP,
				eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_EFFECT_ERROR}, nil,
			)
			continue
		}
		blocked := action.Skill.WindupMS > 0 && b.elapsedMS < target.guardUntil
		damaged := false
		for _, result := range results {
			if result.Kind == pb.EffectType_EFFECT_TYPE_DAMAGE {
				result.Amount = strikeDamage(result.Amount, blocked, counter)
			}
			if event := b.applyResolution(
				actor, target, action.Skill, result,
			); event != nil {
				damaged = true
				if blocked {
					target.guardUntil = 0
					target.counterReady = true
					event.Guarded = true
					event.UpdatedUnit = target.healthSnapshot()
				}
				if counter {
					actor.counterReady = false
					event.Counter = true
					event.UpdatedUnit = actor.healthSnapshot()
				}
			}
		}
		if damaged {
			successfulHits++
		}
	}
	if successfulHits > 0 {
		b.applyCharges(actor, action.Skill, successfulHits)
	}
}

// applyResolution 应用技能效果并返回有效扣血事件，供格挡及反击标注最终结果。
func (b *Battle) applyResolution(
	actor, target *unit, skill SkillConfig, resolution EffectResolution,
) *pb.BattleEvent {
	if !target.alive() {
		return nil
	}
	var damage *pb.BattleEvent
	err := b.strategies.Effects.Apply(EffectApplyContext{
		ActorID: actor.Config.InstanceID, SkillID: skill.ID, Target: effectTarget{unit: target}, Statuses: b.strategies.Statuses,
		ApplyStatus: func(config StatusConfig) error {
			return b.applyStatus(actor, target, skill.ID, config)
		},
		Emit: func(
			kind pb.BattleEventType, statusID string, amount, before, after int64,
			result EffectResolution,
		) {
			info := eventInfo{EffectType: result.Kind, Critical: result.Critical}
			if result.Status != nil {
				info.StatusType = result.Status.Kind
			}
			event := b.emit(
				kind,
				actor.Config.InstanceID, target.Config.InstanceID, skill.ID, statusID,
				amount, before, after,
				info, nil,
			)
			if resolution.Kind == pb.EffectType_EFFECT_TYPE_DAMAGE && kind == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && after < before {
				damage = event
			}
		},
	}, resolution)
	if err != nil {
		b.emit(
			pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
			actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "",
			0, target.HP, target.HP,
			eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_EFFECT_ERROR}, nil,
		)
		return nil
	}
	return damage
}

// applyCharges 在原技能的全部目标结算后，按有效命中数积攒并触发效果。
func (b *Battle) applyCharges(actor *unit, skill SkillConfig, successfulHits int) {
	configured := make(map[pb.EffectType]bool, len(skill.Charges))
	for _, charge := range skill.Charges {
		configured[charge.Kind] = true
		b.applyCharge(actor, skill, charge, successfulHits)
	}
	for _, charge := range actor.Config.PassiveCharges {
		if !configured[charge.Kind] {
			b.applyCharge(actor, skill, charge, successfulHits)
		}
	}
}

// applyCharge 更新单位的效果积攒值并处理触发。
func (b *Battle) applyCharge(actor *unit, skill SkillConfig, charge ChargeEffectConfig, successfulHits int) {
	kind, threshold, gain := charge.Kind, charge.Threshold, charge.Gain
	current := actor.getCharge(kind)
	for hit := 0; hit < successfulHits && current < threshold; hit++ {
		remaining := threshold - current
		if gain >= remaining {
			current = threshold
		} else {
			current += gain
		}
	}
	actor.setCharge(kind, current)
	if !charge.Trigger || current < threshold {
		return
	}
	rule := charge.TargetRule
	if rule == pb.TargetRule_TARGET_RULE_UNSPECIFIED {
		rule = pb.TargetRule_TARGET_RULE_ENEMY_ALL
	}
	targets := b.targets(actor, rule)
	if len(targets) == 0 {
		return
	}
	actor.resetCharge(kind)
	b.triggerCharge(
		actor, skill, charge, targets,
	)
}

// triggerCharge 对当前存活的敌方全体结算冰爆或天雷，二次伤害不再积攒。
func (b *Battle) triggerCharge(
	actor *unit, skill SkillConfig, charge ChargeEffectConfig, targets []*unit,
) {
	actorID, skillID := actor.Config.InstanceID, skill.ID
	for _, target := range targets {
		if !target.alive() {
			continue
		}
		targetID := target.Config.InstanceID
		result, err := b.strategies.Effects.Resolve(EffectContext{
			Actor: b.unitView(actor), Target: b.unitView(target), Skill: skill,
			Effect: charge.TriggerEffect, Rules: b.setup.Rules, Damage: b.strategies.Damage, RNG: b.rng,
		})
		if err != nil {
			b.emit(
				pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED,
				actorID, targetID, skillID, "",
				0, target.HP, target.HP,
				eventInfo{Reason: pb.BattleEventReason_BATTLE_EVENT_REASON_EFFECT_ERROR}, nil,
			)
			continue
		}
		b.applyResolution(
			actor, target, skill, result,
		)
	}
}

type effectTarget struct{ unit *unit }

// ID 返回单位实例编号。
func (t effectTarget) ID() string { return t.unit.Config.InstanceID }

// HP 返回单位当前生命值。
func (t effectTarget) HP() int64 { return t.unit.HP }

// MaxHP 返回单位生命上限。
func (t effectTarget) MaxHP() int64 { return t.unit.Config.MaxHP }

// SetHP 设置单位当前生命值。
func (t effectTarget) SetHP(value int64) { t.unit.HP = value }

// Statuses 返回单位当前的状态列表。
func (t effectTarget) Statuses() []StatusInstance { return t.unit.Statuses }

// SetStatuses 更新单位的状态列表。
func (t effectTarget) SetStatuses(value []StatusInstance) { t.unit.Statuses = value }

// endAction 处理动作结束及后续调度。
func (b *Battle) endAction(action *battleAction) {
	b.expireStatuses(action.Actor)
	b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED)
}

// NextDueMS 返回最近的状态任务、重击命中或战斗截止时刻。
func (b *Battle) NextDueMS() (int64, bool) {
	if b.done || b.finished {
		return 0, false
	}
	deadline := b.setup.Rules.MaxDurationMS
	if b.timeline.Len() == 0 {
		return deadline, true
	}
	return min(deadline, b.timeline[0].ElapsedMs), true
}

// Flush 返回自上次读取事件 之后新产生的事件（例如 Finalize 产生的 BattleEnded），
// 并把游标推到末尾。
func (b *Battle) Flush() []*pb.BattleEvent {
	start := b.emitted
	b.emitted = len(b.events)
	return b.events[start:]
}

// UnitResults 返回当前所有单位的快照。战斗创建时即可调用，
// 让调用方能把参战名单随创建通知一起下发，而不必等到某个单位先行动。
// 注意 Hp/Alive 是"此刻"的值：在战斗开始前调用即为初始状态。
func (b *Battle) UnitResults() []*pb.BattleUnitResult {
	units := make([]*pb.BattleUnitResult, 0, len(b.units))
	for _, unit := range b.units {
		units = append(units, unit.snapshot())
	}
	return units
}

// Finalize 结束战斗并返回确定性的完整结果。重复调用返回首次的结果。
func (b *Battle) Finalize() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	if !b.done {
		if value, done := b.outcome(); done {
			b.finalOutcome = value
		}
	}
	b.action = 0
	b.emit(
		pb.BattleEventType_BATTLE_EVENT_TYPE_BATTLE_ENDED,
		"", "", "", "",
		0, 0, 0,
		eventInfo{Outcome: b.finalOutcome}, nil,
	)
	result := Result{BattleID: b.setup.BattleID, BattleType: b.setup.BattleType, Outcome: b.finalOutcome, ElapsedMs: b.elapsedMS, Units: b.UnitResults(), Events: append([]*pb.BattleEvent(nil), b.events...)}
	checksum, err := resultChecksum(result)
	if err != nil {
		return Result{}, err
	}
	result.Checksum = checksum
	b.finished, b.cached = true, result
	return result, nil
}

// max 返回两个数中的较大值。
func max(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

// min 返回两个数中的较小值。
func min(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

// snapshot 返回单位血量、阵营和归属快照，供开场、入场和终局复用。
func (u *unit) snapshot() *pb.BattleUnitResult {
	snapshot := u.healthSnapshot()
	snapshot.ReceivedGrants = append([]*pb.BattleGrant(nil), u.grants...)
	return snapshot
}

// healthSnapshot 提取血量、架势及重击状态，供事件和完整快照复用。
func (u *unit) healthSnapshot() *pb.BattleUnitResult {
	config := u.Config
	return &pb.BattleUnitResult{
		OwnerUid: config.OwnerUID, InstanceId: config.InstanceID, Team: config.Team,
		Hp: u.HP, MaxHp: config.MaxHP, Alive: u.alive(),
		Windup: u.windup, GuardUntilMs: u.guardUntil, CounterReady: u.counterReady,
	}
}

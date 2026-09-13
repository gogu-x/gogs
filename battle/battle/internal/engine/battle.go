package engine

import (
	"container/heap"
	"fmt"
	"sort"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type activeStatus struct {
	Config    StatusConfig
	Remaining int
}

type unit struct {
	Config    UnitConfig
	HP        int64
	Cooldowns map[string]int
	Statuses  []activeStatus
}

func (u *unit) alive() bool { return u.HP > 0 }

func (u *unit) hasStatus(kind StatusKind) bool {
	for _, status := range u.Statuses {
		if status.Remaining > 0 && status.Config.Kind == kind {
			return true
		}
	}
	return false
}

func (u *unit) modifier() AttributeModifier {
	var modifier AttributeModifier
	for _, status := range u.Statuses {
		if status.Remaining > 0 {
			modifier.AttackFlat += status.Config.Modifier.AttackFlat
			modifier.DefenseFlat += status.Config.Modifier.DefenseFlat
			modifier.SpeedFlat += status.Config.Modifier.SpeedFlat
		}
	}
	return modifier
}

func (u *unit) attack() int64  { return max(0, u.Config.Attack+u.modifier().AttackFlat) }
func (u *unit) defense() int64 { return max(0, u.Config.Defense+u.modifier().DefenseFlat) }
func (u *unit) speed() int64   { return max(1, u.Config.Speed+u.modifier().SpeedFlat) }

// Battle 是不依赖外部服务的确定性战斗实例。
// 全部可变状态都在本结构体上，因此可以按 tick 挂起推进（Advance）而不丢状态。
type Battle struct {
	setup          Setup
	rng            *RNG
	pipeline       DamagePipeline
	units          []*unit
	timeline       timelineQueue
	tick           int64
	action         uint64
	nextAction     uint64
	startedActions int
	activeActions  int
	events         []*pb.BattleEvent
	emitted        int              // events 中已返回给调用方的游标
	finalOutcome   pb.BattleOutcome // 终局判定结果（outcome() 方法已占用该名字）
	terminal       bool             // 是否已判定出终局
	done           bool             // 终止条件已满足，不再推进
	finished       bool
	cached         Result
}

// NewBattle 使用 BattleActor 传入的运行时数据创建战斗。
func NewBattle(setup Setup) (*Battle, error) {
	setup.normalize()
	if err := setup.validate(); err != nil {
		return nil, err
	}
	battle := &Battle{setup: setup, rng: NewRNG(setup.Seed), pipeline: NewDamagePipeline()}
	for _, config := range setup.Units {
		statuses := make([]activeStatus, 0, len(config.InitialStatus))
		for _, status := range config.InitialStatus {
			statuses = append(statuses, activeStatus{Config: status, Remaining: status.DurationTurns})
		}
		battle.units = append(battle.units, &unit{Config: config, HP: config.MaxHP, Cooldowns: make(map[string]int), Statuses: statuses})
	}
	for _, actor := range battle.units {
		battle.schedule(&timelineItem{Tick: battle.attackInterval(actor), Phase: phaseStart, Actor: actor})
	}
	heap.Init(&battle.timeline)
	return battle, nil
}

// ID 返回 Manager 生成的战斗 ID。
func (b *Battle) ID() string { return b.setup.BattleID }

// TickDurationMS 返回客户端播放时间轴使用的权威 tick 时长。
func (b *Battle) TickDurationMS() int32 { return b.setup.Rules.TickDurationMS }

func (b *Battle) emit(kind pb.BattleEventType, actor, target, skill, status string, amount, before, after int64, detail string, targetIDs []string) *pb.BattleEvent {
	event := &pb.BattleEvent{
		Sequence: uint64(len(b.events) + 1), Action: b.action, Tick: b.tick, Type: kind,
		ActorId: actor, TargetId: target, SkillId: skill, StatusId: status,
		Amount: amount, HpBefore: before, HpAfter: after, Detail: detail,
		TargetIds: append([]string(nil), targetIDs...),
	}
	b.events = append(b.events, event)
	return event
}

func (b *Battle) teamAlive(team pb.BattleTeam) bool {
	for _, unit := range b.units {
		if unit.Config.Team == team && unit.alive() {
			return true
		}
	}
	return false
}

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

func (b *Battle) attackInterval(actor *unit) int64 {
	return max(1, (b.setup.Rules.ATBThreshold+actor.speed()-1)/actor.speed())
}

func (b *Battle) tickCooldowns(actor *unit) {
	for id, turns := range actor.Cooldowns {
		if turns <= 1 {
			delete(actor.Cooldowns, id)
		} else {
			actor.Cooldowns[id] = turns - 1
		}
	}
}

func (b *Battle) tickPeriodic(actor *unit) {
	for _, status := range actor.Statuses {
		if !actor.alive() {
			return
		}
		before := actor.HP
		switch status.Config.Kind {
		case StatusDOT:
			amount := min(status.Config.Potency, actor.HP)
			actor.HP -= amount
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, actor.Config.InstanceID, actor.Config.InstanceID, "", status.Config.ID, amount, before, actor.HP, "dot", nil)
		case StatusHOT:
			actor.HP = min(actor.Config.MaxHP, actor.HP+status.Config.Potency)
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, actor.Config.InstanceID, actor.Config.InstanceID, "", status.Config.ID, actor.HP-before, before, actor.HP, "hot", nil)
		}
	}
}

func (b *Battle) expireStatuses(actor *unit) {
	kept := actor.Statuses[:0]
	for _, status := range actor.Statuses {
		status.Remaining--
		if status.Remaining > 0 {
			kept = append(kept, status)
		}
	}
	actor.Statuses = kept
}

func (b *Battle) chooseSkill(actor *unit) SkillConfig {
	if !actor.hasStatus(StatusSilence) {
		for _, skill := range actor.Config.ActiveSkills {
			if actor.Cooldowns[skill.ID] == 0 {
				return skill
			}
		}
	}
	return actor.Config.BasicSkill
}

func unitLess(left, right *unit) bool {
	if left.Config.Position != right.Config.Position {
		return left.Config.Position < right.Config.Position
	}
	return left.Config.InstanceID < right.Config.InstanceID
}

func (b *Battle) targets(actor *unit, rule pb.TargetRule) []*unit {
	var targets []*unit
	switch rule {
	case pb.TargetRule_TARGET_RULE_SELF:
		if actor.alive() {
			targets = append(targets, actor)
		}
	case pb.TargetRule_TARGET_RULE_ENEMY_SINGLE, pb.TargetRule_TARGET_RULE_ENEMY_ALL:
		for _, candidate := range b.units {
			if candidate.alive() && candidate.Config.Team != actor.Config.Team {
				targets = append(targets, candidate)
			}
		}
		sort.Slice(targets, func(i, j int) bool { return unitLess(targets[i], targets[j]) })
		if rule == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE && len(targets) > 1 {
			return targets[:1]
		}
	case pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP:
		for _, candidate := range b.units {
			if candidate.alive() && candidate.Config.Team == actor.Config.Team {
				targets = append(targets, candidate)
			}
		}
		sort.Slice(targets, func(i, j int) bool {
			left, right := targets[i], targets[j]
			if left.HP*right.Config.MaxHP != right.HP*left.Config.MaxHP {
				return left.HP*right.Config.MaxHP < right.HP*left.Config.MaxHP
			}
			return unitLess(left, right)
		})
		if len(targets) > 1 {
			return targets[:1]
		}
	}
	return targets
}

func targetIDs(targets []*unit) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.Config.InstanceID)
	}
	return ids
}

func firstTarget(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (b *Battle) applyEffect(actor, target *unit, skill SkillConfig, effect EffectConfig) {
	if !target.alive() {
		return
	}
	switch effect.Kind {
	case EffectDamage:
		context := b.pipeline.Calculate(DamageContext{Attack: actor.attack(), Defense: target.defense(), CoefficientPermille: effect.CoefficientPermille, Flat: effect.Flat, VariancePermille: b.setup.Rules.DamageVariancePermille, CritChancePermille: b.setup.Rules.CritChancePermille, CritMultiplierPermille: b.setup.Rules.CritMultiplierPermille}, b.rng)
		before := target.HP
		amount := min(context.Amount, target.HP)
		target.HP -= amount
		detail := ""
		if context.Critical {
			detail = "critical"
		}
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "", amount, before, target.HP, detail, nil)
	case EffectHeal:
		amount := max(0, actor.attack()*effect.CoefficientPermille/1000+effect.Flat)
		before := target.HP
		target.HP = min(target.Config.MaxHP, target.HP+amount)
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "", target.HP-before, before, target.HP, "", nil)
	case EffectApplyStatus:
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, effect.StatusID, 0, target.HP, target.HP, "unsupported status config", nil)
	}
}

func (b *Battle) startAction(actor *unit) {
	b.nextAction++
	b.startedActions++
	b.action = b.nextAction
	b.tickCooldowns(actor)
	skill := b.chooseSkill(actor)
	stunned := actor.hasStatus(StatusStun)
	var targets []*unit
	if !stunned {
		targets = b.targets(actor, skill.TargetRule)
	}
	ids := targetIDs(targets)
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED, actor.Config.InstanceID, firstTarget(ids), skill.ID, "", 0, actor.HP, actor.HP, "", ids)
	b.tickPeriodic(actor)

	action := &scheduledAction{ID: b.action, Actor: actor, Skill: skill, Targets: targets, TargetIDs: ids, StartTick: b.tick, Basic: skill.ID == actor.Config.BasicSkill.ID}
	if !actor.alive() {
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_SKIPPED, actor.Config.InstanceID, "", skill.ID, "", 0, actor.HP, actor.HP, "dead_at_start", ids)
		action.ImpactTick, action.EndTick = b.tick, b.tick
	} else if stunned {
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_SKIPPED, actor.Config.InstanceID, "", skill.ID, "", 0, actor.HP, actor.HP, "stun", ids)
		action.ImpactTick, action.EndTick = b.tick, b.tick+skill.RecoveryTicks
	} else {
		detail := "skill"
		if action.Basic {
			detail = "basic"
		}
		// 先算出动作窗口，再随 SKILL_USED 一起下发：客户端收到这一条事件就能确定
		// windup/impact/recovery 并立即起播，不必等待动作结束时的 ACTION_ENDED。
		action.ImpactTick = b.tick + skill.WindupTicks
		action.EndTick = action.ImpactTick + skill.RecoveryTicks
		skillEvent := b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED, actor.Config.InstanceID, firstTarget(ids), skill.ID, "", 0, actor.HP, actor.HP, detail, ids)
		skillEvent.StartTick = action.StartTick
		skillEvent.ImpactTick = action.ImpactTick
		skillEvent.EndTick = action.EndTick
		b.schedule(&timelineItem{Tick: action.ImpactTick, Phase: phaseImpact, Actor: actor, Action: action})
		if skill.Cooldown > 0 && !action.Basic {
			actor.Cooldowns[skill.ID] = skill.Cooldown
		}
	}
	b.activeActions++
	b.schedule(&timelineItem{Tick: action.EndTick, Phase: phaseEnd, Actor: actor, Action: action})
	if actor.alive() {
		nextReady := max(action.EndTick, action.StartTick+b.attackInterval(actor))
		b.schedule(&timelineItem{Tick: nextReady, Phase: phaseStart, Actor: actor})
	}
}

func (b *Battle) impactAction(action *scheduledAction) {
	actor := action.Actor
	if !actor.alive() {
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED, actor.Config.InstanceID, firstTarget(action.TargetIDs), action.Skill.ID, "", 0, actor.HP, actor.HP, "attacker_dead", action.TargetIDs)
		return
	}
	if len(action.Targets) == 0 {
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED, actor.Config.InstanceID, "", action.Skill.ID, "", 0, actor.HP, actor.HP, "target_dead", action.TargetIDs)
		return
	}
	for _, target := range action.Targets {
		if !target.alive() {
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED, actor.Config.InstanceID, target.Config.InstanceID, action.Skill.ID, "", 0, target.HP, target.HP, "target_dead", []string{target.Config.InstanceID})
			continue
		}
		for _, effect := range action.Skill.Effects {
			b.applyEffect(actor, target, action.Skill, effect)
		}
	}
}

func (b *Battle) endAction(action *scheduledAction) {
	b.expireStatuses(action.Actor)
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED, action.Actor.Config.InstanceID, firstTarget(action.TargetIDs), action.Skill.ID, "", 0, action.Actor.HP, action.Actor.HP, "", action.TargetIDs)
	b.activeActions--
}

// NextTick 返回下一个将产生事件的 tick。ok 为 false 表示战斗已结束
// （时间轴已耗尽，或终止条件已满足）。
func (b *Battle) NextTick() (int64, bool) {
	if b.done || b.finished || b.timeline.Len() == 0 {
		return 0, false
	}
	return b.timeline[0].Tick, true
}

// Advance 把模拟推进到当前堆顶 tick（含该 tick 上的全部 phase），返回这批新产生的事件。
// 单次调用不会跨越 tick 边界，因此调用方可以按自己的节奏（例如真实墙钟时间）逐批推送。
// 返回的切片与内部事件切片共享底层数组，调用方须在下一次 Advance 之前消费掉。
func (b *Battle) Advance() []*pb.BattleEvent {
	if b.done || b.finished || b.timeline.Len() == 0 {
		return nil
	}
	start := b.emitted
	target := b.timeline[0].Tick
	for b.timeline.Len() > 0 && b.timeline[0].Tick == target {
		item := heap.Pop(&b.timeline).(*timelineItem)
		b.tick = item.Tick
		switch item.Phase {
		case phaseStart:
			if b.terminal || b.startedActions >= b.setup.Rules.MaxActions || !item.Actor.alive() {
				// 与一次性 Run 的循环保持一致：跳过本轮的终局重算与终止检查。
				continue
			}
			b.startAction(item.Actor)
		case phaseImpact:
			b.action = item.Action.ID
			b.impactAction(item.Action)
		case phaseEnd:
			b.action = item.Action.ID
			b.endAction(item.Action)
		}
		if value, done := b.outcome(); done {
			b.finalOutcome, b.terminal = value, true
		}
		if b.activeActions == 0 && (b.terminal || b.startedActions >= b.setup.Rules.MaxActions) {
			b.done = true
			break
		}
	}
	b.emitted = len(b.events)
	return b.events[start:]
}

// Flush 返回自上次 Advance/Flush 之后新产生的事件（例如 Finalize 产生的 BattleEnded），
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
		units = append(units, &pb.BattleUnitResult{InstanceId: unit.Config.InstanceID, Team: unit.Config.Team, Hp: unit.HP, MaxHp: unit.Config.MaxHP, Alive: unit.alive()})
	}
	return units
}

// Finalize 结束战斗并返回确定性的完整结果。重复调用返回首次的结果。
func (b *Battle) Finalize() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	if !b.terminal {
		if value, done := b.outcome(); done {
			b.finalOutcome = value
		}
	}
	b.action = 0
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_BATTLE_ENDED, "", "", "", "", 0, 0, 0, fmt.Sprintf("outcome=%d", b.finalOutcome), nil)
	result := Result{BattleID: b.setup.BattleID, BattleType: b.setup.BattleType, Outcome: b.finalOutcome, Tick: b.tick, TickDurationMS: b.setup.Rules.TickDurationMS, Units: b.UnitResults(), Events: append([]*pb.BattleEvent(nil), b.events...)}
	checksum, err := resultChecksum(result)
	if err != nil {
		return Result{}, err
	}
	result.Checksum = checksum
	b.finished, b.cached = true, result
	return result, nil
}

// Run 执行一场自动战斗，并返回缓存的确定性结果。
// 它等价于一路 Advance 到底再 Finalize，保留给测试、重投与离线校验使用。
func (b *Battle) Run() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	for {
		if _, ok := b.NextTick(); !ok {
			break
		}
		b.Advance()
	}
	return b.Finalize()
}

func max(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
func min(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

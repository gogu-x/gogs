package engine

import (
	"container/heap"
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type activeStatus = StatusInstance

type unit struct {
	Config    UnitConfig
	HP        int64
	Cooldowns map[string]int
	Statuses  []activeStatus
}

func (u *unit) alive() bool { return u.HP > 0 }

// Battle 是不依赖外部服务的确定性战斗实例。
// 全部可变状态都在本结构体上，因此可以按 tick 挂起推进（Advance）而不丢状态。
type Battle struct {
	setup          Setup
	rng            *RNG
	strategies     Strategies
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
	if err := setup.Strategies.validate(); err != nil {
		return nil, err
	}
	battle := &Battle{setup: setup, rng: NewRNG(setup.Seed), strategies: setup.Strategies}
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
	speed := b.unitSpeed(actor)
	return max(1, (b.setup.Rules.ATBThreshold+speed-1)/speed)
}

func (b *Battle) unitView(u *unit) UnitView {
	return UnitView{ID: u.Config.InstanceID, Team: u.Config.Team, Position: u.Config.Position, HP: u.HP, MaxHP: u.Config.MaxHP,
		Attack: b.unitAttack(u), Defense: b.unitDefense(u), Alive: u.alive()}
}

func (b *Battle) unitAttack(u *unit) int64 {
	return max(0, u.Config.Attack+b.unitModifier(u).AttackFlat)
}
func (b *Battle) unitDefense(u *unit) int64 {
	return max(0, u.Config.Defense+b.unitModifier(u).DefenseFlat)
}
func (b *Battle) unitSpeed(u *unit) int64 { return max(1, u.Config.Speed+b.unitModifier(u).SpeedFlat) }
func (b *Battle) unitModifier(u *unit) AttributeModifier {
	return b.strategies.Statuses.Modifier(u.Statuses)
}
func (b *Battle) hasStatus(u *unit, kind statuskind.Kind) bool {
	return b.strategies.Statuses.Has(u.Statuses, kind)
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
		result := b.strategies.Statuses.Tick(status)
		if result.Damage > 0 {
			result.Damage = min(result.Damage, actor.HP)
			actor.HP -= result.Damage
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, actor.Config.InstanceID, actor.Config.InstanceID, "", status.Config.ID, result.Damage, before, actor.HP, result.Event, nil)
		}
		if result.Heal > 0 {
			actor.HP = min(actor.Config.MaxHP, actor.HP+result.Heal)
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, actor.Config.InstanceID, actor.Config.InstanceID, "", status.Config.ID, actor.HP-before, before, actor.HP, result.Event, nil)
		}
	}
}

func (b *Battle) expireStatuses(actor *unit) {
	actor.Statuses = b.strategies.Statuses.Expire(actor.Statuses)
}

func (b *Battle) chooseSkill(actor *unit) SkillConfig {
	if !b.hasStatus(actor, statuskind.Silence) {
		for _, skill := range actor.Config.ActiveSkills {
			if actor.Cooldowns[skill.ID] == 0 {
				return skill
			}
		}
	}
	return actor.Config.BasicSkill
}

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

func (b *Battle) startAction(actor *unit) {
	b.nextAction++
	b.startedActions++
	b.action = b.nextAction
	b.tickCooldowns(actor)
	skill := b.chooseSkill(actor)
	stunned := b.hasStatus(actor, statuskind.Stun)
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
		results, err := b.strategies.Skills.Execute(SkillExecutionContext{
			Actor: b.unitView(actor), Target: b.unitView(target), Skill: action.Skill,
			Rules: b.setup.Rules, Effects: b.strategies.Effects, Damage: b.strategies.Damage, RNG: b.rng,
		})
		if err != nil {
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED, actor.Config.InstanceID, target.Config.InstanceID, action.Skill.ID, "", 0, target.HP, target.HP, err.Error(), nil)
			continue
		}
		for _, result := range results {
			b.applyResolution(actor, target, action.Skill, result)
		}
	}
}

func (b *Battle) applyResolution(actor, target *unit, skill SkillConfig, resolution EffectResolution) {
	if !target.alive() {
		return
	}
	err := b.strategies.Effects.Apply(EffectApplyContext{
		ActorID: actor.Config.InstanceID, SkillID: skill.ID, Target: effectTarget{unit: target}, Statuses: b.strategies.Statuses,
		Emit: func(kind pb.BattleEventType, statusID string, amount, before, after int64, detail string) {
			b.emit(kind, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, statusID, amount, before, after, detail, nil)
		},
	}, resolution)
	if err != nil {
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_CANCELLED, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "", 0, target.HP, target.HP, err.Error(), nil)
	}
}

type effectTarget struct{ unit *unit }

func (t effectTarget) ID() string                         { return t.unit.Config.InstanceID }
func (t effectTarget) HP() int64                          { return t.unit.HP }
func (t effectTarget) MaxHP() int64                       { return t.unit.Config.MaxHP }
func (t effectTarget) SetHP(value int64)                  { t.unit.HP = value }
func (t effectTarget) Statuses() []StatusInstance         { return t.unit.Statuses }
func (t effectTarget) SetStatuses(value []StatusInstance) { t.unit.Statuses = value }

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

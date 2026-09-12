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

func (b *Battle) emit(kind pb.BattleEventType, actor, target, skill, status string, amount, before, after int64, detail string, targetIDs []string) {
	b.events = append(b.events, &pb.BattleEvent{
		Sequence: uint64(len(b.events) + 1), Action: b.action, Tick: b.tick, Type: kind,
		ActorId: actor, TargetId: target, SkillId: skill, StatusId: status,
		Amount: amount, HpBefore: before, HpAfter: after, Detail: detail,
		TargetIds: append([]string(nil), targetIDs...),
	})
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
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED, actor.Config.InstanceID, firstTarget(ids), skill.ID, "", 0, actor.HP, actor.HP, detail, ids)
		action.ImpactTick = b.tick + skill.WindupTicks
		action.EndTick = action.ImpactTick + skill.RecoveryTicks
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

// Run 执行一场自动战斗，并返回缓存的确定性结果。
func (b *Battle) Run() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	outcome := pb.BattleOutcome_BATTLE_OUTCOME_DRAW
	terminal := false
	for b.timeline.Len() > 0 {
		item := heap.Pop(&b.timeline).(*timelineItem)
		b.tick = item.Tick
		switch item.Phase {
		case phaseStart:
			if terminal || b.startedActions >= b.setup.Rules.MaxActions || !item.Actor.alive() {
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
			outcome, terminal = value, true
		}
		if b.activeActions == 0 && (terminal || b.startedActions >= b.setup.Rules.MaxActions) {
			break
		}
	}
	if !terminal {
		if value, done := b.outcome(); done {
			outcome = value
		}
	}
	b.action = 0
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_BATTLE_ENDED, "", "", "", "", 0, 0, 0, fmt.Sprintf("outcome=%d", outcome), nil)
	units := make([]*pb.BattleUnitResult, 0, len(b.units))
	for _, unit := range b.units {
		units = append(units, &pb.BattleUnitResult{InstanceId: unit.Config.InstanceID, Team: unit.Config.Team, Hp: unit.HP, MaxHp: unit.Config.MaxHP, Alive: unit.alive()})
	}
	result := Result{BattleID: b.setup.BattleID, Outcome: outcome, Tick: b.tick, TickDurationMS: b.setup.Rules.TickDurationMS, Units: units, Events: append([]*pb.BattleEvent(nil), b.events...)}
	checksum, err := resultChecksum(result)
	if err != nil {
		return Result{}, err
	}
	result.Checksum = checksum
	b.finished, b.cached = true, result
	return result, nil
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

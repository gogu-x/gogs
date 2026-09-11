package engine

import (
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
	Gauge     int64
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
	setup    Setup
	rng      *RNG
	pipeline DamagePipeline
	units    []*unit
	tick     int64
	action   uint64
	events   []*pb.BattleEvent
	finished bool
	cached   Result
}

// NewBattle 使用 BattleActor 传入的运行时数据创建战斗。
func NewBattle(setup Setup) (*Battle, error) {
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
	return battle, nil
}

// ID 返回 Manager 生成的战斗 ID。
func (b *Battle) ID() string { return b.setup.BattleID }

func (b *Battle) emit(kind pb.BattleEventType, actor, target, skill, status string, amount, before, after int64, detail string) {
	b.events = append(b.events, &pb.BattleEvent{
		Sequence: uint64(len(b.events) + 1), Action: b.action, Tick: b.tick, Type: kind,
		ActorId: actor, TargetId: target, SkillId: skill, StatusId: status,
		Amount: amount, HpBefore: before, HpAfter: after, Detail: detail,
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

func (b *Battle) nextActor() *unit {
	var delta int64 = -1
	for _, unit := range b.units {
		if !unit.alive() {
			continue
		}
		need := b.setup.Rules.ATBThreshold - unit.Gauge
		ticks := int64(0)
		if need > 0 {
			ticks = (need + unit.speed() - 1) / unit.speed()
		}
		if delta < 0 || ticks < delta {
			delta = ticks
		}
	}
	if delta < 0 {
		return nil
	}
	if delta > 0 {
		for _, unit := range b.units {
			if unit.alive() {
				unit.Gauge += delta * unit.speed()
			}
		}
		b.tick += delta
	}
	ready := make([]*unit, 0)
	for _, unit := range b.units {
		if unit.alive() && unit.Gauge >= b.setup.Rules.ATBThreshold {
			ready = append(ready, unit)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		left, right := ready[i], ready[j]
		if left.Gauge != right.Gauge {
			return left.Gauge > right.Gauge
		}
		if left.speed() != right.speed() {
			return left.speed() > right.speed()
		}
		if left.Config.Position != right.Config.Position {
			return left.Config.Position < right.Config.Position
		}
		return left.Config.InstanceID < right.Config.InstanceID
	})
	if len(ready) == 0 {
		return nil
	}
	return ready[0]
}

func (b *Battle) tickCooldowns(unit *unit) {
	for id, turns := range unit.Cooldowns {
		if turns <= 1 {
			delete(unit.Cooldowns, id)
		} else {
			unit.Cooldowns[id] = turns - 1
		}
	}
}

func (b *Battle) tickPeriodic(unit *unit) {
	for _, status := range unit.Statuses {
		if !unit.alive() {
			return
		}
		before := unit.HP
		switch status.Config.Kind {
		case StatusDOT:
			amount := min(status.Config.Potency, unit.HP)
			unit.HP -= amount
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, unit.Config.InstanceID, unit.Config.InstanceID, "", status.Config.ID, amount, before, unit.HP, "dot")
		case StatusHOT:
			unit.HP = min(unit.Config.MaxHP, unit.HP+status.Config.Potency)
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, unit.Config.InstanceID, unit.Config.InstanceID, "", status.Config.ID, unit.HP-before, before, unit.HP, "hot")
		}
	}
}

func (b *Battle) expireStatuses(unit *unit) {
	kept := unit.Statuses[:0]
	for _, status := range unit.Statuses {
		status.Remaining--
		if status.Remaining > 0 {
			kept = append(kept, status)
		}
	}
	unit.Statuses = kept
}

func (b *Battle) chooseSkill(unit *unit) SkillConfig {
	if !unit.hasStatus(StatusSilence) {
		for _, skill := range unit.Config.ActiveSkills {
			if unit.Cooldowns[skill.ID] == 0 {
				return skill
			}
		}
	}
	return unit.Config.BasicSkill
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
		for _, unit := range b.units {
			if unit.alive() && unit.Config.Team != actor.Config.Team {
				targets = append(targets, unit)
			}
		}
		sort.Slice(targets, func(i, j int) bool { return unitLess(targets[i], targets[j]) })
		if rule == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE && len(targets) > 1 {
			return targets[:1]
		}
	case pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP:
		for _, unit := range b.units {
			if unit.alive() && unit.Config.Team == actor.Config.Team {
				targets = append(targets, unit)
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
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "", amount, before, target.HP, detail)
	case EffectHeal:
		amount := max(0, actor.attack()*effect.CoefficientPermille/1000+effect.Flat)
		before := target.HP
		target.HP = min(target.Config.MaxHP, target.HP+amount)
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, "", target.HP-before, before, target.HP, "")
	case EffectApplyStatus:
		b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, actor.Config.InstanceID, target.Config.InstanceID, skill.ID, effect.StatusID, 0, target.HP, target.HP, "unsupported status config")
	}
}

func (b *Battle) execute(actor *unit) {
	b.action++
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED, actor.Config.InstanceID, "", "", "", 0, actor.HP, actor.HP, "")
	b.tickCooldowns(actor)
	b.tickPeriodic(actor)
	if actor.alive() {
		if actor.hasStatus(StatusStun) {
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_SKIPPED, actor.Config.InstanceID, "", "", "", 0, actor.HP, actor.HP, "stun")
		} else {
			skill := b.chooseSkill(actor)
			targets := b.targets(actor, skill.TargetRule)
			b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED, actor.Config.InstanceID, "", skill.ID, "", 0, actor.HP, actor.HP, "")
			for _, target := range targets {
				for _, effect := range skill.Effects {
					b.applyEffect(actor, target, skill, effect)
				}
			}
			if skill.Cooldown > 0 && skill.ID != actor.Config.BasicSkill.ID {
				actor.Cooldowns[skill.ID] = skill.Cooldown
			}
		}
	}
	b.expireStatuses(actor)
	actor.Gauge = max(0, actor.Gauge-b.setup.Rules.ATBThreshold)
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED, actor.Config.InstanceID, "", "", "", 0, actor.HP, actor.HP, "")
}

// Run 执行一场自动战斗，并返回缓存的确定性结果。
func (b *Battle) Run() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	outcome := pb.BattleOutcome_BATTLE_OUTCOME_DRAW
	for action := 0; action < b.setup.Rules.MaxActions; action++ {
		if value, done := b.outcome(); done {
			outcome = value
			break
		}
		actor := b.nextActor()
		if actor == nil {
			return Result{}, fmt.Errorf("no living battle unit available")
		}
		b.execute(actor)
	}
	if value, done := b.outcome(); done {
		outcome = value
	}
	b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_BATTLE_ENDED, "", "", "", "", 0, 0, 0, fmt.Sprintf("outcome=%d", outcome))
	units := make([]*pb.BattleUnitResult, 0, len(b.units))
	for _, unit := range b.units {
		units = append(units, &pb.BattleUnitResult{InstanceId: unit.Config.InstanceID, Team: unit.Config.Team, Hp: unit.HP, MaxHp: unit.Config.MaxHP, Alive: unit.alive()})
	}
	result := Result{BattleID: b.setup.BattleID, Outcome: outcome, Tick: b.tick, Units: units, Events: append([]*pb.BattleEvent(nil), b.events...)}
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

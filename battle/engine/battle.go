package engine

import (
	"fmt"
	"sort"
)

type activeStatus struct {
	Config    StatusConfig `json:"config"`
	Remaining int          `json:"remaining"`
}

type unit struct {
	Input     CombatantInput
	Config    UnitConfig
	HP        int64
	Gauge     int64
	Cooldowns map[string]int
	Statuses  []activeStatus
}

func (u *unit) alive() bool { return u.HP > 0 }
func (u *unit) hasStatus(kind StatusKind) bool {
	for _, s := range u.Statuses {
		if s.Remaining > 0 && s.Config.Kind == kind {
			return true
		}
	}
	return false
}
func (u *unit) modifier() AttributeModifier {
	var out AttributeModifier
	for _, s := range u.Statuses {
		if s.Remaining > 0 {
			out.AttackFlat += s.Config.Modifier.AttackFlat
			out.DefenseFlat += s.Config.Modifier.DefenseFlat
			out.SpeedFlat += s.Config.Modifier.SpeedFlat
		}
	}
	return out
}
func (u *unit) attack() int64 {
	v := u.Config.Attack + u.modifier().AttackFlat
	if v < 0 {
		return 0
	}
	return v
}
func (u *unit) defense() int64 {
	v := u.Config.Defense + u.modifier().DefenseFlat
	if v < 0 {
		return 0
	}
	return v
}
func (u *unit) speed() int64 {
	v := u.Config.Speed + u.modifier().SpeedFlat
	if v < 1 {
		return 1
	}
	return v
}

type Battle struct {
	id         string
	input      BattleInput
	config     Config
	configHash string
	rng        *RNG
	pipeline   DamagePipeline
	units      []*unit
	tick       int64
	action     uint64
	events     []Event
	finished   bool
	cached     Result
}

func NewBattle(repo ConfigRepository, input BattleInput) (*Battle, error) {
	if repo == nil {
		return nil, invalid("config_repository", "required")
	}
	config, err := repo.Get(input.ConfigVersion)
	if err != nil {
		return nil, err
	}
	if err := input.Validate(config); err != nil {
		return nil, err
	}
	input = canonicalInput(input)
	inputHash, err := StableInputHash(input)
	if err != nil {
		return nil, err
	}
	configHash, err := StableConfigHash(config)
	if err != nil {
		return nil, err
	}
	idHash, err := hashJSON(struct{ InputHash, ConfigHash string }{inputHash, configHash})
	if err != nil {
		return nil, err
	}
	b := &Battle{id: idHash[:32], input: input, config: config, configHash: configHash, rng: NewRNG(input.Seed), pipeline: NewDamagePipeline()}
	for _, p := range input.Combatants {
		u := config.Units[p.ConfigID]
		b.units = append(b.units, &unit{Input: p, Config: u, HP: u.MaxHP, Cooldowns: map[string]int{}})
	}
	return b, nil
}

func (b *Battle) ID() string { return b.id }

func (b *Battle) emit(typ EventType, actor, target, skill, status string, amount, before, after int64, detail string) {
	b.events = append(b.events, Event{Sequence: uint64(len(b.events) + 1), Action: b.action, Tick: b.tick, Type: typ, ActorID: actor, TargetID: target, SkillID: skill, StatusID: status, Amount: amount, HPBefore: before, HPAfter: after, Detail: detail})
}

func (b *Battle) teamAlive(team Team) bool {
	for _, u := range b.units {
		if u.Input.Team == team && u.alive() {
			return true
		}
	}
	return false
}
func (b *Battle) outcome() (Outcome, bool) {
	a, d := b.teamAlive(TeamAttacker), b.teamAlive(TeamDefender)
	if a && d {
		return 0, false
	}
	if a {
		return OutcomeAttackerWin, true
	}
	if d {
		return OutcomeDefenderWin, true
	}
	return OutcomeDraw, true
}

func (b *Battle) nextActor() *unit {
	var delta int64 = -1
	for _, u := range b.units {
		if !u.alive() {
			continue
		}
		need := b.config.ATBThreshold - u.Gauge
		var d int64
		if need > 0 {
			d = (need + u.speed() - 1) / u.speed()
		}
		if delta < 0 || d < delta {
			delta = d
		}
	}
	if delta < 0 {
		return nil
	}
	if delta > 0 {
		for _, u := range b.units {
			if u.alive() {
				u.Gauge += delta * u.speed()
			}
		}
		b.tick += delta
	}
	ready := make([]*unit, 0)
	for _, u := range b.units {
		if u.alive() && u.Gauge >= b.config.ATBThreshold {
			ready = append(ready, u)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		a, c := ready[i], ready[j]
		if a.Gauge != c.Gauge {
			return a.Gauge > c.Gauge
		}
		if a.speed() != c.speed() {
			return a.speed() > c.speed()
		}
		if a.Input.Position != c.Input.Position {
			return a.Input.Position < c.Input.Position
		}
		return a.Input.InstanceID < c.Input.InstanceID
	})
	if len(ready) == 0 {
		return nil
	}
	return ready[0]
}

func (b *Battle) tickCooldowns(u *unit) {
	for id, n := range u.Cooldowns {
		if n <= 1 {
			delete(u.Cooldowns, id)
		} else {
			u.Cooldowns[id] = n - 1
		}
	}
}

func (b *Battle) tickPeriodic(u *unit) {
	for _, s := range u.Statuses {
		if !u.alive() {
			break
		}
		switch s.Config.Kind {
		case StatusDOT:
			before := u.HP
			amount := s.Config.Potency
			if amount > u.HP {
				amount = u.HP
			}
			u.HP -= amount
			b.emit(EventDamage, u.Input.InstanceID, u.Input.InstanceID, "", s.Config.ID, amount, before, u.HP, "dot")
		case StatusHOT:
			before := u.HP
			u.HP += s.Config.Potency
			if u.HP > u.Config.MaxHP {
				u.HP = u.Config.MaxHP
			}
			amount := u.HP - before
			b.emit(EventHeal, u.Input.InstanceID, u.Input.InstanceID, "", s.Config.ID, amount, before, u.HP, "hot")
		}
	}
}

func (b *Battle) expireStatuses(u *unit) {
	kept := u.Statuses[:0]
	for _, s := range u.Statuses {
		s.Remaining--
		if s.Remaining > 0 {
			kept = append(kept, s)
		}
	}
	u.Statuses = kept
}

func (b *Battle) chooseSkill(u *unit) SkillConfig {
	if !u.hasStatus(StatusSilence) {
		for _, id := range u.Config.ActiveSkillIDs {
			if u.Cooldowns[id] == 0 {
				return b.config.Skills[id]
			}
		}
	}
	return b.config.Skills[u.Config.BasicSkillID]
}

func unitLess(a, c *unit) bool {
	if a.Input.Position != c.Input.Position {
		return a.Input.Position < c.Input.Position
	}
	return a.Input.InstanceID < c.Input.InstanceID
}

func (b *Battle) targets(actor *unit, rule TargetRule) []*unit {
	var out []*unit
	switch rule {
	case TargetSelf:
		if actor.alive() {
			out = []*unit{actor}
		}
	case TargetEnemySingle, TargetEnemyAll:
		for _, u := range b.units {
			if u.alive() && u.Input.Team != actor.Input.Team {
				out = append(out, u)
			}
		}
		sort.Slice(out, func(i, j int) bool { return unitLess(out[i], out[j]) })
		if rule == TargetEnemySingle && len(out) > 1 {
			out = out[:1]
		}
	case TargetAllyLowestHP:
		for _, u := range b.units {
			if u.alive() && u.Input.Team == actor.Input.Team {
				out = append(out, u)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			a, c := out[i], out[j]
			left, right := a.HP*c.Config.MaxHP, c.HP*a.Config.MaxHP
			if left != right {
				return left < right
			}
			return unitLess(a, c)
		})
		if len(out) > 1 {
			out = out[:1]
		}
	}
	return out
}

func (b *Battle) applyEffect(actor, target *unit, skill SkillConfig, e EffectConfig) {
	if !target.alive() {
		return
	}
	switch e.Kind {
	case EffectDamage:
		ctx := b.pipeline.Calculate(DamageContext{Attack: actor.attack(), Defense: target.defense(), CoefficientPermille: e.CoefficientPermille, Flat: e.Flat, VariancePermille: b.config.DamageVariancePermille, CritChancePermille: b.config.CritChancePermille, CritMultiplierPermille: b.config.CritMultiplierPermille}, b.rng)
		before := target.HP
		amount := ctx.Amount
		if amount > target.HP {
			amount = target.HP
		}
		target.HP -= amount
		detail := ""
		if ctx.Critical {
			detail = "critical"
		}
		b.emit(EventDamage, actor.Input.InstanceID, target.Input.InstanceID, skill.ID, "", amount, before, target.HP, detail)
	case EffectHeal:
		amount := actor.attack()*e.CoefficientPermille/1000 + e.Flat
		if amount < 0 {
			amount = 0
		}
		before := target.HP
		target.HP += amount
		if target.HP > target.Config.MaxHP {
			target.HP = target.Config.MaxHP
		}
		amount = target.HP - before
		b.emit(EventHeal, actor.Input.InstanceID, target.Input.InstanceID, skill.ID, "", amount, before, target.HP, "")
	case EffectApplyStatus:
		status := b.config.Statuses[e.StatusID]
		found := false
		for i := range target.Statuses {
			if target.Statuses[i].Config.ID == status.ID {
				target.Statuses[i] = activeStatus{Config: status, Remaining: status.DurationTurns}
				found = true
				break
			}
		}
		if !found {
			target.Statuses = append(target.Statuses, activeStatus{Config: status, Remaining: status.DurationTurns})
		}
		b.emit(EventStatusApplied, actor.Input.InstanceID, target.Input.InstanceID, skill.ID, status.ID, 0, target.HP, target.HP, fmt.Sprintf("kind=%d duration=%d", status.Kind, status.DurationTurns))
	}
}

func (b *Battle) execute(actor *unit) {
	b.action++
	b.emit(EventActionStarted, actor.Input.InstanceID, "", "", "", 0, actor.HP, actor.HP, "")
	b.tickCooldowns(actor)
	b.tickPeriodic(actor)
	if actor.alive() {
		if actor.hasStatus(StatusStun) {
			b.emit(EventActionSkipped, actor.Input.InstanceID, "", "", "", 0, actor.HP, actor.HP, "stun")
		} else {
			skill := b.chooseSkill(actor)
			targets := b.targets(actor, skill.TargetRule)
			b.emit(EventSkillUsed, actor.Input.InstanceID, "", skill.ID, "", 0, actor.HP, actor.HP, "")
			for _, effect := range skill.Effects {
				for _, target := range targets {
					b.applyEffect(actor, target, skill, effect)
				}
			}
			if skill.Cooldown > 0 && skill.ID != actor.Config.BasicSkillID {
				actor.Cooldowns[skill.ID] = skill.Cooldown
			}
		}
	}
	b.expireStatuses(actor)
	actor.Gauge -= b.config.ATBThreshold
	if actor.Gauge < 0 {
		actor.Gauge = 0
	}
	b.emit(EventActionEnded, actor.Input.InstanceID, "", "", "", 0, actor.HP, actor.HP, "")
}

func (b *Battle) Run() (Result, error) {
	if b.finished {
		return b.cached, nil
	}
	var outcome Outcome
	for actions := 0; actions < b.config.MaxActions; actions++ {
		if o, done := b.outcome(); done {
			outcome = o
			break
		}
		actor := b.nextActor()
		if actor == nil {
			return Result{}, fmt.Errorf("no living actor available")
		}
		b.execute(actor)
	}
	if outcome == 0 {
		if o, done := b.outcome(); done {
			outcome = o
		} else {
			outcome = OutcomeDraw
		}
	}
	b.emit(EventBattleEnded, "", "", "", "", 0, 0, 0, fmt.Sprintf("outcome=%d", outcome))
	units := make([]UnitResult, 0, len(b.units))
	for _, u := range b.units {
		units = append(units, UnitResult{InstanceID: u.Input.InstanceID, Team: u.Input.Team, HP: u.HP, MaxHP: u.Config.MaxHP, Alive: u.alive()})
	}
	result := Result{BattleID: b.id, Outcome: outcome, Tick: b.tick, Units: units, Events: append([]Event(nil), b.events...)}
	checksum, err := b.resultChecksum(result)
	if err != nil {
		return Result{}, err
	}
	result.Checksum = checksum
	result.Replay = Replay{BattleID: b.id, ConfigVersion: b.config.Version, ConfigHash: b.configHash, Input: b.input, Events: append([]Event(nil), b.events...), Checksum: checksum}
	b.finished = true
	b.cached = result
	return result, nil
}

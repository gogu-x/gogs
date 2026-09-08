package engine

import (
	"fmt"
	"sync"
)

func (c Config) Validate() error {
	if c.Version == "" {
		return invalid("config.version", "required")
	}
	if c.ATBThreshold <= 0 {
		return invalid("config.atb_threshold", "must be positive")
	}
	if c.MaxActions <= 0 {
		return invalid("config.max_actions", "must be positive")
	}
	if c.DamageVariancePermille < 0 || c.DamageVariancePermille > 1000 {
		return invalid("config.damage_variance_permille", "must be in [0,1000]")
	}
	if c.CritChancePermille < 0 || c.CritChancePermille > 1000 {
		return invalid("config.crit_chance_permille", "must be in [0,1000]")
	}
	if c.CritMultiplierPermille < 1000 {
		return invalid("config.crit_multiplier_permille", "must be at least 1000")
	}
	if len(c.Units) == 0 || len(c.Skills) == 0 {
		return invalid("config", "units and skills are required")
	}
	for id, s := range c.Statuses {
		if id == "" || s.ID != id {
			return invalid("status.id", "map key and id must match")
		}
		if s.Kind < StatusStun || s.Kind > StatusAttributeModifier {
			return invalid("status.kind", "unknown")
		}
		if s.DurationTurns <= 0 {
			return invalid("status.duration_turns", "must be positive")
		}
		if (s.Kind == StatusDOT || s.Kind == StatusHOT) && s.Potency <= 0 {
			return invalid("status.potency", "must be positive for periodic status")
		}
	}
	for id, s := range c.Skills {
		if id == "" || s.ID != id {
			return invalid("skill.id", "map key and id must match")
		}
		if s.Cooldown < 0 || !s.TargetRule.Valid() || len(s.Effects) == 0 {
			return invalid("skill", fmt.Sprintf("%s has invalid cooldown, target, or effects", id))
		}
		for _, e := range s.Effects {
			switch e.Kind {
			case EffectDamage, EffectHeal:
				if e.CoefficientPermille < 0 {
					return invalid("effect.coefficient_permille", "must not be negative")
				}
			case EffectApplyStatus:
				if _, ok := c.Statuses[e.StatusID]; !ok {
					return invalid("effect.status_id", "not found")
				}
			default:
				return invalid("effect.kind", "unknown")
			}
		}
	}
	for id, u := range c.Units {
		if id == "" || u.ID != id {
			return invalid("unit.id", "map key and id must match")
		}
		if u.MaxHP <= 0 || u.Attack < 0 || u.Defense < 0 || u.Speed <= 0 {
			return invalid("unit.attributes", fmt.Sprintf("%s has invalid attributes", id))
		}
		if _, ok := c.Skills[u.BasicSkillID]; !ok {
			return invalid("unit.basic_skill_id", "not found")
		}
		for _, skillID := range u.ActiveSkillIDs {
			if _, ok := c.Skills[skillID]; !ok {
				return invalid("unit.active_skill_ids", "skill not found")
			}
		}
	}
	return nil
}

func (in BattleInput) Validate(c Config) error {
	if in.ConfigVersion == "" || in.ConfigVersion != c.Version {
		return invalid("input.config_version", "does not match config")
	}
	if len(in.Combatants) < 2 {
		return invalid("input.combatants", "at least two required")
	}
	ids := make(map[string]struct{}, len(in.Combatants))
	positions := make(map[string]struct{}, len(in.Combatants))
	teams := map[Team]int{}
	for _, p := range in.Combatants {
		if p.InstanceID == "" {
			return invalid("combatant.instance_id", "required")
		}
		if _, exists := ids[p.InstanceID]; exists {
			return invalid("combatant.instance_id", "duplicate")
		}
		ids[p.InstanceID] = struct{}{}
		if !p.Team.Valid() {
			return invalid("combatant.team", "unknown")
		}
		if p.Position < 0 {
			return invalid("combatant.position", "must not be negative")
		}
		positionKey := fmt.Sprintf("%d/%d", p.Team, p.Position)
		if _, exists := positions[positionKey]; exists {
			return invalid("combatant.position", "duplicate within team")
		}
		positions[positionKey] = struct{}{}
		if _, ok := c.Units[p.ConfigID]; !ok {
			return invalid("combatant.config_id", "not found")
		}
		teams[p.Team]++
	}
	if teams[TeamAttacker] == 0 || teams[TeamDefender] == 0 {
		return invalid("input.combatants", "both teams required")
	}
	return nil
}

func cloneConfig(c Config) Config {
	out := c
	out.Units = make(map[string]UnitConfig, len(c.Units))
	for k, v := range c.Units {
		v.ActiveSkillIDs = append([]string(nil), v.ActiveSkillIDs...)
		out.Units[k] = v
	}
	out.Skills = make(map[string]SkillConfig, len(c.Skills))
	for k, v := range c.Skills {
		v.Effects = append([]EffectConfig(nil), v.Effects...)
		out.Skills[k] = v
	}
	out.Statuses = make(map[string]StatusConfig, len(c.Statuses))
	for k, v := range c.Statuses {
		out.Statuses[k] = v
	}
	return out
}

type ConfigRepository interface {
	Put(Config) error
	Get(version string) (Config, error)
}

type MemoryConfigRepository struct {
	mu       sync.RWMutex
	versions map[string]Config
	hashes   map[string]string
}

func NewMemoryConfigRepository() *MemoryConfigRepository {
	return &MemoryConfigRepository{versions: map[string]Config{}, hashes: map[string]string{}}
}

func (r *MemoryConfigRepository) Put(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	hash, err := StableConfigHash(c)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, exists := r.hashes[c.Version]; exists && old != hash {
		return fmt.Errorf("config version %q already exists with different content", c.Version)
	}
	r.versions[c.Version] = cloneConfig(c)
	r.hashes[c.Version] = hash
	return nil
}

func (r *MemoryConfigRepository) Get(version string) (Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.versions[version]
	if !ok {
		return Config{}, fmt.Errorf("config version %q not found", version)
	}
	return cloneConfig(c), nil
}

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func canonicalInput(in BattleInput) BattleInput {
	out := in
	out.Combatants = append([]CombatantInput(nil), in.Combatants...)
	sort.Slice(out.Combatants, func(i, j int) bool {
		a, b := out.Combatants[i], out.Combatants[j]
		if a.Team != b.Team {
			return a.Team < b.Team
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.InstanceID < b.InstanceID
	})
	return out
}

func hashJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func StableInputHash(in BattleInput) (string, error) { return hashJSON(canonicalInput(in)) }

type configCanonical struct {
	Version                string         `json:"version"`
	ATBThreshold           int64          `json:"atb_threshold"`
	MaxActions             int            `json:"max_actions"`
	DamageVariancePermille int64          `json:"damage_variance_permille"`
	CritChancePermille     int64          `json:"crit_chance_permille"`
	CritMultiplierPermille int64          `json:"crit_multiplier_permille"`
	Units                  []UnitConfig   `json:"units"`
	Skills                 []SkillConfig  `json:"skills"`
	Statuses               []StatusConfig `json:"statuses"`
}

func StableConfigHash(c Config) (string, error) {
	v := configCanonical{Version: c.Version, ATBThreshold: c.ATBThreshold, MaxActions: c.MaxActions, DamageVariancePermille: c.DamageVariancePermille, CritChancePermille: c.CritChancePermille, CritMultiplierPermille: c.CritMultiplierPermille}
	for _, x := range c.Units {
		x.ActiveSkillIDs = append([]string(nil), x.ActiveSkillIDs...)
		v.Units = append(v.Units, x)
	}
	for _, x := range c.Skills {
		x.Effects = append([]EffectConfig(nil), x.Effects...)
		v.Skills = append(v.Skills, x)
	}
	for _, x := range c.Statuses {
		v.Statuses = append(v.Statuses, x)
	}
	sort.Slice(v.Units, func(i, j int) bool { return v.Units[i].ID < v.Units[j].ID })
	sort.Slice(v.Skills, func(i, j int) bool { return v.Skills[i].ID < v.Skills[j].ID })
	sort.Slice(v.Statuses, func(i, j int) bool { return v.Statuses[i].ID < v.Statuses[j].ID })
	return hashJSON(v)
}

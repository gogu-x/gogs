package battlecfg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/gogu-x/gogs/battle/iproto"
)

// CanonicalInput 规范化输入（排序参战单位），保证同内容同指纹，是
// StableInputHash 与结算侧构造 BattleID 共同依赖的单一实现。
func CanonicalInput(in iproto.BattleInput) iproto.BattleInput {
	out := in
	out.Combatants = append([]iproto.CombatantInput(nil), in.Combatants...)
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

// HashJSON 对任意值做确定性 JSON + SHA-256。导出供结算侧 checksum 复用，
// 避免在其它包重复实现导致指纹漂移。
func HashJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func StableInputHash(in iproto.BattleInput) (string, error) { return HashJSON(CanonicalInput(in)) }

type configCanonical struct {
	Version                string                `json:"version"`
	ATBThreshold           int64                 `json:"atb_threshold"`
	MaxActions             int                   `json:"max_actions"`
	DamageVariancePermille int64                 `json:"damage_variance_permille"`
	CritChancePermille     int64                 `json:"crit_chance_permille"`
	CritMultiplierPermille int64                 `json:"crit_multiplier_permille"`
	Units                  []iproto.UnitConfig   `json:"units"`
	Skills                 []iproto.SkillConfig  `json:"skills"`
	Statuses               []iproto.StatusConfig `json:"statuses"`
}

func StableConfigHash(c iproto.Config) (string, error) {
	v := configCanonical{Version: c.Version, ATBThreshold: c.ATBThreshold, MaxActions: c.MaxActions, DamageVariancePermille: c.DamageVariancePermille, CritChancePermille: c.CritChancePermille, CritMultiplierPermille: c.CritMultiplierPermille}
	for _, x := range c.Units {
		x.ActiveSkillIDs = append([]string(nil), x.ActiveSkillIDs...)
		v.Units = append(v.Units, x)
	}
	for _, x := range c.Skills {
		x.Effects = append([]iproto.EffectConfig(nil), x.Effects...)
		v.Skills = append(v.Skills, x)
	}
	for _, x := range c.Statuses {
		v.Statuses = append(v.Statuses, x)
	}
	sort.Slice(v.Units, func(i, j int) bool { return v.Units[i].ID < v.Units[j].ID })
	sort.Slice(v.Skills, func(i, j int) bool { return v.Skills[i].ID < v.Skills[j].ID })
	sort.Slice(v.Statuses, func(i, j int) bool { return v.Statuses[i].ID < v.Statuses[j].ID })
	return HashJSON(v)
}

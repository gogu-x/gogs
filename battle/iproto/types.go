// Package battlecfg 承载战斗引擎的配置数据面与输入面类型、校验、稳定哈希
// 与多版本配置仓库。它是公共配置模块，独立于 battle 服务与结算引擎，
// 任何进程（结算、校验、对账、工具）都可复用，且只依赖标准库。
package iproto

type Team uint8

const (
	TeamAttacker Team = 1
	TeamDefender Team = 2
)

func (t Team) Valid() bool { return t == TeamAttacker || t == TeamDefender }

type TargetRule uint8

const (
	TargetEnemySingle TargetRule = iota + 1
	TargetEnemyAll
	TargetAllyLowestHP
	TargetSelf
)

func (r TargetRule) Valid() bool { return r >= TargetEnemySingle && r <= TargetSelf }

type EffectKind uint8

const (
	EffectDamage EffectKind = iota + 1
	EffectHeal
	EffectApplyStatus
)

type StatusKind uint8

const (
	StatusStun StatusKind = iota + 1
	StatusSilence
	StatusDOT
	StatusHOT
	StatusAttributeModifier
)

type AttributeModifier struct {
	AttackFlat  int64 `json:"attack_flat"`
	DefenseFlat int64 `json:"defense_flat"`
	SpeedFlat   int64 `json:"speed_flat"`
}

type StatusConfig struct {
	ID            string            `json:"id"`
	Kind          StatusKind        `json:"kind"`
	DurationTurns int               `json:"duration_turns"`
	Potency       int64             `json:"potency"`
	Modifier      AttributeModifier `json:"modifier"`
}

type EffectConfig struct {
	Kind                EffectKind `json:"kind"`
	CoefficientPermille int64      `json:"coefficient_permille"`
	Flat                int64      `json:"flat"`
	StatusID            string     `json:"status_id"`
}

type SkillConfig struct {
	ID         string         `json:"id"`
	Cooldown   int            `json:"cooldown"`
	TargetRule TargetRule     `json:"target_rule"`
	Effects    []EffectConfig `json:"effects"`
}

type UnitConfig struct {
	ID             string   `json:"id"`
	MaxHP          int64    `json:"max_hp"`
	Attack         int64    `json:"attack"`
	Defense        int64    `json:"defense"`
	Speed          int64    `json:"speed"`
	BasicSkillID   string   `json:"basic_skill_id"`
	ActiveSkillIDs []string `json:"active_skill_ids"`
}

type Config struct {
	Version                string                  `json:"version"`
	ATBThreshold           int64                   `json:"atb_threshold"`
	MaxActions             int                     `json:"max_actions"`
	DamageVariancePermille int64                   `json:"damage_variance_permille"`
	CritChancePermille     int64                   `json:"crit_chance_permille"`
	CritMultiplierPermille int64                   `json:"crit_multiplier_permille"`
	Units                  map[string]UnitConfig   `json:"units"`
	Skills                 map[string]SkillConfig  `json:"skills"`
	Statuses               map[string]StatusConfig `json:"statuses"`
}

type CombatantInput struct {
	InstanceID string `json:"instance_id"`
	ConfigID   string `json:"config_id"`
	Team       Team   `json:"team"`
	Position   int    `json:"position"`
}

type BattleInput struct {
	ConfigVersion string           `json:"config_version"`
	Seed          uint64           `json:"seed"`
	Combatants    []CombatantInput `json:"combatants"`
}

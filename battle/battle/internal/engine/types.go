package engine

import (
	"fmt"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// AttributeModifier contains flat changes contributed by active statuses.
type AttributeModifier struct {
	AttackFlat  int64
	DefenseFlat int64
	SpeedFlat   int64
}

// StatusConfig contains the resolved runtime data for one status.
type StatusConfig struct {
	ID            string
	Kind          pb.StatusType
	DurationTurns int
	Potency       int64
	Modifier      AttributeModifier
}

// EffectConfig contains the resolved runtime data for one skill effect.
type EffectConfig struct {
	Kind                pb.EffectType
	CoefficientPermille int64
	Flat                int64
	StatusID            string
	Status              *StatusConfig
}

// DefaultTickDurationMS 是旧规则未配置 tick 时长时使用的兼容默认值。
const DefaultTickDurationMS int32 = 100

// SkillConfig 表示运行时技能配置。
type SkillConfig struct {
	ID            string
	Cooldown      int
	TargetRule    pb.TargetRule
	Effects       []EffectConfig
	Charges       []ChargeEffectConfig // 本技能每次有效命中对出手单位积攒值的贡献。
	WindupTicks   int64
	RecoveryTicks int64
}

// ChargeEffectConfig 表示一次有效命中对出手单位积攒值的贡献和满值效果。
type ChargeEffectConfig struct {
	Kind          pb.EffectType // 冰爆或天雷效果类型。
	Threshold     int64         // 触发所需的积攒值。
	Gain          int64         // 每命中一个目标增加的积攒值。
	Trigger       bool          // 当前技能是否能在满值时触发。
	TargetRule    pb.TargetRule // 满值效果的目标规则；未指定时为敌方全体。
	TriggerEffect EffectConfig  // 积攒满值时执行的效果配置。
}

// UnitConfig 表示 BattleActor 已从 glconf 还原的运行时单位。
type UnitConfig struct {
	InstanceID     string
	Team           pb.BattleTeam
	Position       int32
	MaxHP          int64
	Attack         int64
	Defense        int64
	Speed          int64
	BasicSkill     SkillConfig
	ActiveSkills   []SkillConfig
	InitialStatus  []StatusConfig
	PassiveCharges []ChargeEffectConfig // 该角色装备魂玉提供的积攒效果。
}

// Rules 表示 BattleActor 已从 glconf 读取的本场规则。
type Rules struct {
	ATBThreshold           int64
	MaxActions             int
	TickDurationMS         int32
	DamageVariancePermille int64
	CritChancePermille     int64
	CritMultiplierPermille int64
}

// Setup 是 Engine 的完整纯运行时输入。
type Setup struct {
	BattleID string
	// BattleType 只做透传：引擎的模拟与战斗类型无关，但结果里必须带上它 ——
	// Game 结算要按类型分发，战报重投也要能知道原来打的是什么。
	BattleType pb.BattleType
	Seed       uint64
	Rules      Rules
	Units      []UnitConfig
	Strategies Strategies
}

// Result 是确定性战斗的运行结果。
type Result struct {
	BattleID       string                 `json:"battle_id"`
	BattleType     pb.BattleType          `json:"battle_type"`
	Outcome        pb.BattleOutcome       `json:"outcome"`
	Tick           int64                  `json:"tick"`
	TickDurationMS int32                  `json:"tick_duration_ms"`
	Units          []*pb.BattleUnitResult `json:"units"`
	Events         []*pb.BattleEvent      `json:"events"`
	Checksum       string                 `json:"checksum"`
}

func (s *Setup) normalize() {
	if s.Rules.TickDurationMS <= 0 {
		s.Rules.TickDurationMS = DefaultTickDurationMS
	}
}

func (s Setup) validate() error {
	if s.BattleID == "" {
		return invalid("battle_id", "required")
	}
	if s.Rules.ATBThreshold <= 0 {
		return invalid("atb_threshold", "must be positive")
	}
	if s.Rules.MaxActions <= 0 {
		return invalid("max_actions", "must be positive")
	}
	if len(s.Units) < 2 {
		return invalid("units", "at least two units required")
	}
	teams := make(map[pb.BattleTeam]bool)
	ids := make(map[string]struct{}, len(s.Units))
	for _, unit := range s.Units {
		if unit.InstanceID == "" {
			return invalid("unit.instance_id", "required")
		}
		if _, exists := ids[unit.InstanceID]; exists {
			return invalid("unit.instance_id", fmt.Sprintf("duplicate %q", unit.InstanceID))
		}
		ids[unit.InstanceID] = struct{}{}
		if unit.Team != pb.BattleTeam_BATTLE_TEAM_ATTACKER && unit.Team != pb.BattleTeam_BATTLE_TEAM_DEFENDER {
			return invalid("unit.team", "must be attacker or defender")
		}
		teams[unit.Team] = true
		if unit.MaxHP <= 0 || unit.Speed <= 0 {
			return invalid("unit.attributes", "max hp and speed must be positive")
		}
		if unit.BasicSkill.ID == "" {
			return invalid("unit.basic_skill", "required")
		}
		allSkills := append([]SkillConfig{unit.BasicSkill}, unit.ActiveSkills...)
		thresholds := make(map[pb.EffectType]int64)
		seenPassive := make(map[pb.EffectType]bool)
		for _, charge := range unit.PassiveCharges {
			if err := validateChargeEffect(charge); err != nil {
				return err
			}
			if seenPassive[charge.Kind] {
				return invalid("unit.passive_charge.kind", "duplicate kind")
			}
			seenPassive[charge.Kind] = true
			thresholds[charge.Kind] = charge.Threshold
		}
		for _, skill := range allSkills {
			if skill.WindupTicks < 0 || skill.RecoveryTicks < 0 {
				return invalid("unit.skill.timing", "windup and recovery ticks cannot be negative")
			}
			if len(skill.Charges) > 0 {
				hasDamage := false
				for _, effect := range skill.Effects {
					if effect.Kind == pb.EffectType_EFFECT_TYPE_DAMAGE {
						hasDamage = true
						break
					}
				}
				if !hasDamage {
					return invalid("unit.skill.charge", "charged skill requires a damage effect")
				}
			}
			seenCharges := make(map[pb.EffectType]bool)
			for _, charge := range skill.Charges {
				kind, threshold := charge.Kind, charge.Threshold
				if err := validateChargeEffect(charge); err != nil {
					return err
				}
				if seenCharges[kind] {
					return invalid("unit.skill.charge.kind", "duplicate kind in one skill")
				}
				seenCharges[kind] = true
				if existing := thresholds[kind]; existing != 0 && existing != threshold {
					return invalid("unit.skill.charge.threshold", "same kind must share one threshold per unit")
				}
				thresholds[kind] = threshold
			}
		}
	}
	if !teams[pb.BattleTeam_BATTLE_TEAM_ATTACKER] || !teams[pb.BattleTeam_BATTLE_TEAM_DEFENDER] {
		return invalid("units", "both teams required")
	}
	return nil
}

func validateChargeEffect(charge ChargeEffectConfig) error {
	if charge.Kind != pb.EffectType_EFFECT_TYPE_ICE_BURST && charge.Kind != pb.EffectType_EFFECT_TYPE_THUNDER {
		return invalid("unit.charge.kind", "must be ice burst or thunder")
	}
	if charge.Threshold <= 0 || charge.Gain < 0 || (!charge.Trigger && charge.Gain == 0) {
		return invalid("unit.charge", "threshold must be positive and charge must gain or trigger")
	}
	if charge.Trigger && (charge.TriggerEffect.Kind != charge.Kind || charge.TriggerEffect.CoefficientPermille <= 0) {
		return invalid("unit.charge.trigger_effect", "trigger requires a matching effect and positive coefficient")
	}
	switch charge.TargetRule {
	case pb.TargetRule_TARGET_RULE_UNSPECIFIED:
	case pb.TargetRule_TARGET_RULE_ENEMY_SINGLE:
	case pb.TargetRule_TARGET_RULE_ENEMY_ALL:
	default:
		return invalid("unit.charge.target_rule", "must target enemies")
	}
	return nil
}

func invalid(field, reason string) error { return fmt.Errorf("invalid %s: %s", field, reason) }

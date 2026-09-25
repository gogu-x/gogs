package engine

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
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
	Kind          statuskind.Kind
	DurationTurns int
	Potency       int64
	Modifier      AttributeModifier
}

// EffectConfig contains the resolved runtime data for one skill effect.
type EffectConfig struct {
	Kind                effectkind.Kind
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
	WindupTicks   int64
	RecoveryTicks int64
}

// UnitConfig 表示 BattleActor 已从 glconf 还原的运行时单位。
type UnitConfig struct {
	InstanceID    string
	Team          pb.BattleTeam
	Position      int32
	MaxHP         int64
	Attack        int64
	Defense       int64
	Speed         int64
	BasicSkill    SkillConfig
	ActiveSkills  []SkillConfig
	InitialStatus []StatusConfig
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
		for _, skill := range allSkills {
			if skill.WindupTicks < 0 || skill.RecoveryTicks < 0 {
				return invalid("unit.skill.timing", "windup and recovery ticks cannot be negative")
			}
		}
	}
	if !teams[pb.BattleTeam_BATTLE_TEAM_ATTACKER] || !teams[pb.BattleTeam_BATTLE_TEAM_DEFENDER] {
		return invalid("units", "both teams required")
	}
	return nil
}

func invalid(field, reason string) error { return fmt.Errorf("invalid %s: %s", field, reason) }

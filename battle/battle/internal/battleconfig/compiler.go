package battleconfig

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/battlestatus"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/mode"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect"
	"github.com/gogu-x/gogs/battle/battle/internal/targeting"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Compiler resolves a battle request and its configuration into an immutable
// engine setup. The engine never reads game configuration during simulation.
type Compiler struct {
	modes             mode.Registry
	roleStats         RoleStatCalculator
	skills            RoleSkillProgression
	strategies        engine.Strategies
	effectDefinitions skilleffect.Definitions
}

// New 根据战斗模式注册表创建配置编译器。
func New(modes mode.Registry) *Compiler {
	registry := make(mode.Registry, len(modes))
	for battleType, rule := range modes {
		if rule != nil {
			registry[battleType] = rule
		}
	}
	return &Compiler{modes: registry, roleStats: configuredRoleStatCalculator{}, skills: configuredRoleSkillProgression{}, strategies: defaultStrategies(), effectDefinitions: skilleffect.DefaultDefinitions()}
}

// defaultStrategies 组装引擎使用的内置战斗策略。
func defaultStrategies() engine.Strategies {
	return engine.Strategies{Targets: targeting.Default(), Skills: engine.DefaultSkillExecutor{}, Effects: skilleffect.Default(), Damage: engine.DefaultDamageFormula{}, Statuses: battlestatus.Default()}
}

// DefaultStrategies 返回引擎的默认战斗策略。
func DefaultStrategies() engine.Strategies { return defaultStrategies() }

// WithStrategies 使用指定策略覆盖默认实现，未指定的策略沿用默认值。
func (c *Compiler) WithStrategies(strategies engine.Strategies) *Compiler {
	if c == nil {
		return nil
	}
	copy := *c
	defaults := defaultStrategies()
	if strategies.Targets != nil {
		defaults.Targets = strategies.Targets
	}
	if strategies.Skills != nil {
		defaults.Skills = strategies.Skills
	}
	if strategies.Effects != nil {
		defaults.Effects = strategies.Effects
	}
	if strategies.Damage != nil {
		defaults.Damage = strategies.Damage
	}
	if strategies.Statuses != nil {
		defaults.Statuses = strategies.Statuses
	}
	copy.strategies = defaults
	return &copy
}

// WithEffectDefinition 复制效果定义并在新编译器中注册指定效果。
func (c *Compiler) WithEffectDefinition(name string, definition skilleffect.Definition) *Compiler {
	if c == nil {
		return nil
	}
	copy := *c
	copy.effectDefinitions = make(skilleffect.Definitions, len(c.effectDefinitions)+1)
	for key, value := range c.effectDefinitions {
		copy.effectDefinitions[key] = value
	}
	copy.effectDefinitions[name] = definition
	return &copy
}

// NewDefault 使用内置战斗模式创建配置编译器。
func NewDefault() *Compiler { return New(mode.DefaultRegistry()) }

// NewWithRoleStats 创建使用指定角色属性算法的编译器。
func NewWithRoleStats(modes mode.Registry, calculator RoleStatCalculator) *Compiler {
	c := New(modes)
	if calculator != nil {
		c.roleStats = calculator
	}
	return c
}

// NewWithProgression 创建使用指定属性与技能成长算法的编译器。
func NewWithProgression(modes mode.Registry, roleStats RoleStatCalculator, skills RoleSkillProgression) *Compiler {
	c := NewWithRoleStats(modes, roleStats)
	if skills != nil {
		c.skills = skills
	}
	return c
}

// Compile 将创建请求和配置编译为本场战斗的初始数据。
func (c *Compiler) Compile(battleID string, seed uint64, request *pb.StartBattleReq) (engine.Setup, error) {
	if request == nil {
		return engine.Setup{}, fmt.Errorf("battle request is required")
	}
	if c == nil {
		return engine.Setup{}, fmt.Errorf("battle config compiler is not initialized")
	}
	battleType := request.GetBattleType()
	rule, ok := c.modes[battleType]
	if !ok {
		return engine.Setup{}, fmt.Errorf("unsupported battle type=%d", battleType)
	}
	scenario, err := rule.Resolve(request)
	if err != nil {
		return engine.Setup{}, err
	}
	battleRule := glconf.GetBattleRuleCfg(int32(battleType))
	if battleRule == nil {
		return engine.Setup{}, fmt.Errorf("battle rule config not found: battle_type=%d", battleType)
	}
	units := make([]engine.UnitConfig, 0, len(scenario.Attackers)+len(scenario.Defenders)+4)
	attackerPositions := make(map[int32]struct{}, len(scenario.Attackers))
	for _, role := range scenario.Attackers {
		unit, err := c.BuildEncounterRole(role, battleType)
		if err != nil {
			return engine.Setup{}, err
		}
		unit.OwnerUID = request.GetUID()
		if err := addUnitPosition(attackerPositions, unit); err != nil {
			return engine.Setup{}, err
		}
		units = append(units, unit)
	}

	opponentPositions := make(map[int32]struct{})
	if scenario.MonsterGroupConfigID != 0 {
		members, err := glconf.GetBattleMonsterMembers(scenario.MonsterGroupConfigID)
		if err != nil {
			return engine.Setup{}, err
		}
		for _, member := range members {
			if err := member.GetError(); err != nil {
				return engine.Setup{}, err
			}
			position := member.GetPosition()
			opponentPositions[position] = struct{}{}
			unit, err := c.buildMonsterUnit(member.GetID(), position, battleType)
			if err != nil {
				return engine.Setup{}, err
			}
			if err := configureMonsterAI(member.GetID(), &unit); err != nil {
				return engine.Setup{}, err
			}
			units = append(units, unit)
		}
	}
	for _, role := range scenario.Defenders {
		unit, err := c.buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_DEFENDER)
		if err != nil {
			return engine.Setup{}, err
		}
		if _, duplicate := opponentPositions[unit.Position]; duplicate {
			return engine.Setup{}, fmt.Errorf("duplicate configured defender position=%d", unit.Position)
		}
		opponentPositions[unit.Position] = struct{}{}
		units = append(units, unit)
	}

	return engine.Setup{BattleID: battleID, BattleType: battleType, Seed: seed, Rules: engine.Rules{
		ATBThreshold: battleRule.ATBThreshold, MaxDurationMS: int64(battleRule.MaxDurationMS),
		DamageVariancePermille: battleRule.DamageVariancePermille, CritChancePermille: battleRule.CritChancePermille,
		CritMultiplierPermille: battleRule.CritMultiplierPermille,
	}, Units: units, Strategies: c.strategies}, nil
}

// BuildRoleUnit 单独编译角色单位，创建整场战斗时通过 Compile 完成完整校验。
func (c *Compiler) BuildRoleUnit(role *pb.Role, team pb.BattleTeam) (engine.UnitConfig, error) {
	return c.buildRoleUnit(role, team)
}

// BuildSkill 单独编译指定配置编号的技能。
func (c *Compiler) BuildSkill(id int32) (engine.SkillConfig, error) { return c.buildSkill(id) }

// addUnitPosition 登记单位站位并检查是否重复。
func addUnitPosition(positions map[int32]struct{}, unit engine.UnitConfig) error {
	if _, exists := positions[unit.Position]; exists {
		return fmt.Errorf("duplicate configured attacker position=%d", unit.Position)
	}
	positions[unit.Position] = struct{}{}
	return nil
}

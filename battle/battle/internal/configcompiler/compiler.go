package configcompiler

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/damageformula"
	"github.com/gogu-x/gogs/battle/battle/internal/effectresolver"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/mode"
	"github.com/gogu-x/gogs/battle/battle/internal/skillexecutor"
	"github.com/gogu-x/gogs/battle/battle/internal/statushandler"
	"github.com/gogu-x/gogs/battle/battle/internal/targetselector"
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
	effectDefinitions effectresolver.Definitions
}

func New(modes mode.Registry) *Compiler {
	registry := make(mode.Registry, len(modes))
	for battleType, rule := range modes {
		if rule != nil {
			registry[battleType] = rule
		}
	}
	return &Compiler{modes: registry, roleStats: configuredRoleStatCalculator{}, skills: configuredRoleSkillProgression{}, strategies: defaultStrategies(), effectDefinitions: effectresolver.DefaultDefinitions()}
}

func defaultStrategies() engine.Strategies {
	return engine.Strategies{Targets: targetselector.Default(), Skills: skillexecutor.Default{}, Effects: effectresolver.Default(), Damage: damageformula.Default{}, Statuses: statushandler.Default()}
}

func DefaultStrategies() engine.Strategies { return defaultStrategies() }

// WithStrategies returns a compiler that uses the supplied battle policies.
// Nil policies inherit their default implementations.
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

// WithEffectDefinitions extends config effect type decoding. Existing
// definitions are copied, so registering a new type does not mutate peers.
func (c *Compiler) WithEffectDefinition(name string, definition effectresolver.Definition) *Compiler {
	if c == nil {
		return nil
	}
	copy := *c
	copy.effectDefinitions = make(effectresolver.Definitions, len(c.effectDefinitions)+1)
	for key, value := range c.effectDefinitions {
		copy.effectDefinitions[key] = value
	}
	copy.effectDefinitions[name] = definition
	return &copy
}

func NewDefault() *Compiler { return New(mode.DefaultRegistry()) }

func NewWithRoleStats(modes mode.Registry, calculator RoleStatCalculator) *Compiler {
	c := New(modes)
	if calculator != nil {
		c.roleStats = calculator
	}
	return c
}

func NewWithProgression(modes mode.Registry, roleStats RoleStatCalculator, skills RoleSkillProgression) *Compiler {
	c := NewWithRoleStats(modes, roleStats)
	if skills != nil {
		c.skills = skills
	}
	return c
}

func (c *Compiler) Compile(battleID string, seed uint64, request *pb.StartBattleReq) (engine.Setup, error) {
	if request == nil {
		return engine.Setup{}, fmt.Errorf("battle request is required")
	}
	if c == nil {
		return engine.Setup{}, fmt.Errorf("battle config compiler is not initialized")
	}
	rule, ok := c.modes[request.GetBattleType()]
	if !ok {
		return engine.Setup{}, fmt.Errorf("unsupported battle type=%d", request.GetBattleType())
	}
	scenario, err := rule.Resolve(request)
	if err != nil {
		return engine.Setup{}, err
	}
	battleRule := glconf.GetBattleRuleCfg(int32(request.GetBattleType()))
	if battleRule == nil {
		return engine.Setup{}, fmt.Errorf("battle rule config not found: battle_type=%d", request.GetBattleType())
	}
	units := make([]engine.UnitConfig, 0, len(scenario.Attackers)+len(scenario.Defenders)+4)
	attackerPositions := make(map[int32]struct{}, len(scenario.Attackers))
	for _, role := range scenario.Attackers {
		unit, err := c.buildRoleUnit(role, pb.BattleTeam_BATTLE_TEAM_ATTACKER)
		if err != nil {
			return engine.Setup{}, err
		}
		if err := addUnitPosition(attackerPositions, unit); err != nil {
			return engine.Setup{}, err
		}
		units = append(units, unit)
	}

	opponentPositions := make(map[int32]struct{})
	if scenario.MonsterGroupConfigID != 0 {
		group := glconf.GetBattleMonsterGroupCfg(scenario.MonsterGroupConfigID)
		if group == nil {
			return engine.Setup{}, fmt.Errorf("battle monster group config not found: group_config_id=%d", scenario.MonsterGroupConfigID)
		}
		if len(group.Members) == 0 {
			return engine.Setup{}, fmt.Errorf("battle monster group config id=%d has no members", group.CfgID)
		}
		for _, member := range group.Members {
			_, position, err := monsterMemberValues(member)
			if err != nil {
				return engine.Setup{}, fmt.Errorf("battle monster group config id=%d: %w", group.CfgID, err)
			}
			if _, duplicate := opponentPositions[position]; duplicate {
				return engine.Setup{}, fmt.Errorf("battle monster group config id=%d has duplicate position=%d", group.CfgID, position)
			}
			opponentPositions[position] = struct{}{}
			unit, err := c.buildMonsterUnit(member)
			if err != nil {
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

	return engine.Setup{BattleID: battleID, BattleType: request.GetBattleType(), Seed: seed, Rules: engine.Rules{
		ATBThreshold: battleRule.ATBThreshold, MaxActions: int(battleRule.MaxActions), TickDurationMS: battleRule.TickDurationMS,
		DamageVariancePermille: battleRule.DamageVariancePermille, CritChancePermille: battleRule.CritChancePermille,
		CritMultiplierPermille: battleRule.CritMultiplierPermille,
	}, Units: units, Strategies: c.strategies}, nil
}

// BuildRoleUnit compiles one role for legacy package-level helpers and focused
// tooling. New integrations should call Compile so all validation is applied.
func (c *Compiler) BuildRoleUnit(role *pb.Role, team pb.BattleTeam) (engine.UnitConfig, error) {
	return c.buildRoleUnit(role, team)
}

// BuildSkill compiles one configured skill.
func (c *Compiler) BuildSkill(id int32) (engine.SkillConfig, error) { return c.buildSkill(id) }

func addUnitPosition(positions map[int32]struct{}, unit engine.UnitConfig) error {
	if _, exists := positions[unit.Position]; exists {
		return fmt.Errorf("duplicate configured attacker position=%d", unit.Position)
	}
	positions[unit.Position] = struct{}{}
	return nil
}

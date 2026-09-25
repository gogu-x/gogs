package engine

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// UnitView is an immutable snapshot passed to pluggable battle policies.
type UnitView struct {
	ID       string
	Team     pb.BattleTeam
	Position int32
	HP       int64
	MaxHP    int64
	Attack   int64
	Defense  int64
	Alive    bool
}

type TargetQuery struct {
	Actor UnitView
	Units []UnitView
	Rule  pb.TargetRule
}

// TargetSelector maps configured target rules to stable unit IDs.
type TargetSelector interface{ Select(TargetQuery) []string }

type DamageInput struct {
	Attack, Defense, CoefficientPermille, Flat                   int64
	VariancePermille, CritChancePermille, CritMultiplierPermille int64
}
type DamageResult struct {
	Amount   int64
	Critical bool
}
type DamageFormula interface {
	Calculate(DamageInput, *RNG) DamageResult
}

type StatusInstance struct {
	Config    StatusConfig
	Remaining int
}
type StatusTickResult struct {
	Damage, Heal int64
	Event        string
}

// StatusHandler owns status lookup, application, periodic ticks and expiry.
type StatusHandler interface {
	Has([]StatusInstance, statuskind.Kind) bool
	Modifier([]StatusInstance) AttributeModifier
	Apply([]StatusInstance, StatusConfig) ([]StatusInstance, error)
	Tick(StatusInstance) StatusTickResult
	Expire([]StatusInstance) []StatusInstance
}

// StatusPolicy owns the behavior for one status kind within a registry.
type StatusPolicy interface {
	Apply([]StatusInstance, StatusConfig) ([]StatusInstance, error)
	Tick(StatusInstance) StatusTickResult
}

type EffectContext struct {
	Actor, Target UnitView
	Skill         SkillConfig
	Effect        EffectConfig
	Rules         Rules
	Damage        DamageFormula
	RNG           *RNG
}
type EffectResolution struct {
	Kind     effectkind.Kind
	Amount   int64
	Critical bool
	Status   *StatusConfig
	Detail   string
}

// EffectResolver calculates one effect; Battle remains responsible for state
// mutation and event sequencing.
type EffectResolver interface {
	Resolve(EffectContext) (EffectResolution, error)
	Apply(EffectApplyContext, EffectResolution) error
}

type EffectTarget interface {
	ID() string
	HP() int64
	MaxHP() int64
	SetHP(int64)
	Statuses() []StatusInstance
	SetStatuses([]StatusInstance)
}

type EffectApplyContext struct {
	ActorID  string
	SkillID  string
	Target   EffectTarget
	Statuses StatusHandler
	Emit     func(pb.BattleEventType, string, int64, int64, int64, string)
}

type SkillExecutionContext struct {
	Actor, Target UnitView
	Skill         SkillConfig
	Rules         Rules
	Effects       EffectResolver
	Damage        DamageFormula
	RNG           *RNG
}

// SkillExecutor expands a selected skill into resolved effects for one target.
// Engine applies results and owns timeline/event ordering.
type SkillExecutor interface {
	Execute(SkillExecutionContext) ([]EffectResolution, error)
}

type Strategies struct {
	Targets  TargetSelector
	Skills   SkillExecutor
	Effects  EffectResolver
	Damage   DamageFormula
	Statuses StatusHandler
}

func (s Strategies) validate() error {
	if s.Targets == nil {
		return fmt.Errorf("battle strategy is not configured: target selector")
	}
	if s.Skills == nil {
		return fmt.Errorf("battle strategy is not configured: skill executor")
	}
	if s.Effects == nil {
		return fmt.Errorf("battle strategy is not configured: effect resolver")
	}
	if s.Damage == nil {
		return fmt.Errorf("battle strategy is not configured: damage formula")
	}
	if s.Statuses == nil {
		return fmt.Errorf("battle strategy is not configured: status handler")
	}
	return nil
}

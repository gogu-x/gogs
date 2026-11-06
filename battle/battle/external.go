package battle

import (
	"github.com/gogu-x/gogs/battle/battle/internal"
	"github.com/gogu-x/gogs/battle/battle/internal/battleconfig"
	"github.com/gogu-x/gogs/battle/battle/internal/battlestatus"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/mode"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect"
	"github.com/gogu-x/gogs/battle/battle/internal/targeting"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

type Params = internal.Params
type Mode = internal.Mode
type BattleScenario = mode.Scenario
type BattleMode = mode.Rule
type ConfigCompiler = battleconfig.Compiler
type RoleAttributes = battleconfig.RoleAttributes
type RoleStatCalculator = battleconfig.RoleStatCalculator
type RoleSkillProgression = battleconfig.RoleSkillProgression
type BattleStrategies = engine.Strategies
type TargetSelector = engine.TargetSelector
type TargetPolicy = targeting.Selector
type TargetPolicyFunc = targeting.SelectorFunc
type TargetRegistry = targeting.Registry
type TargetQuery = engine.TargetQuery
type BattleUnitView = engine.UnitView
type EffectResolver = engine.EffectResolver

type EffectHandler = skilleffect.Handler
type EffectHandlerFunc = skilleffect.HandlerFunc
type EffectHandlerFuncs = skilleffect.HandlerFuncs
type EffectRegistry = skilleffect.Registry
type EffectDefinition = skilleffect.Definition
type EffectDefinitions = skilleffect.Definitions
type SkillExecutor = engine.SkillExecutor
type SkillExecutionContext = engine.SkillExecutionContext
type EffectContext = engine.EffectContext
type EffectApplyContext = engine.EffectApplyContext
type EffectTarget = engine.EffectTarget
type EffectResolution = engine.EffectResolution
type DamageFormula = engine.DamageFormula
type DamageInput = engine.DamageInput
type DamageResult = engine.DamageResult
type StatusHandler = engine.StatusHandler

type StatusRegistry = battlestatus.Registry
type StatusPolicy = engine.StatusPolicy
type StatusInstance = engine.StatusInstance
type StatusTickResult = engine.StatusTickResult

type Source = internal.Source
type Notifier = internal.Notifier
type NotifyFunc = internal.NotifyFunc
type Repository = internal.Repository
type ActorStopped = internal.ActorStopped

// New 创建并返回一个独立运行的战斗 Actor。
func New(p Params) tree.Actor { return internal.NewBattleActor(p) }

// NewDefaultConfigCompiler 创建使用默认玩法规则和战斗策略的配置编译器。
func NewDefaultConfigCompiler() *ConfigCompiler { return battleconfig.NewDefault() }

// NewDefaultSkillExecutor 返回按配置顺序解析普通技能效果的公共执行器。
func NewDefaultSkillExecutor() SkillExecutor { return engine.DefaultSkillExecutor{} }

// NewConfigCompiler 创建使用指定战斗模式的配置编译器。
func NewConfigCompiler(modes map[pb.BattleType]BattleMode) *ConfigCompiler {
	return battleconfig.New(mode.Registry(modes))
}

// NewConfigCompilerWithRoleStats 创建带有自定义角色属性计算器的配置编译器。
func NewConfigCompilerWithRoleStats(modes map[pb.BattleType]BattleMode, calculator RoleStatCalculator) *ConfigCompiler {
	return battleconfig.NewWithRoleStats(mode.Registry(modes), calculator)
}

// NewConfigCompilerWithProgression 创建带有自定义属性和技能成长规则的配置编译器。
func NewConfigCompilerWithProgression(modes map[pb.BattleType]BattleMode, roleStats RoleStatCalculator, skills RoleSkillProgression) *ConfigCompiler {
	return battleconfig.NewWithProgression(mode.Registry(modes), roleStats, skills)
}

// DefaultBattleModes 返回默认战斗模式注册表的副本。
func DefaultBattleModes() map[pb.BattleType]BattleMode {
	registry := mode.DefaultRegistry()
	modes := make(map[pb.BattleType]BattleMode, len(registry))
	for battleType, rule := range registry {
		modes[battleType] = rule
	}
	return modes
}

// NewMemoryRepository 创建用于本地运行或测试的内存战报仓储。
func NewMemoryRepository() Repository { return internal.NewMemoryRepository() }

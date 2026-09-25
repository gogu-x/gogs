package battle

import (
	"github.com/gogu-x/gogs/battle/battle/internal"
	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/effectresolver"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/mode"
	"github.com/gogu-x/gogs/battle/battle/internal/statushandler"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
	"github.com/gogu-x/gogs/battle/battle/internal/targetselector"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

type Params = internal.Params
type Mode = internal.Mode
type BattleScenario = mode.Scenario
type BattleMode = mode.Rule
type ConfigCompiler = configcompiler.Compiler
type RoleAttributes = configcompiler.RoleAttributes
type RoleStatCalculator = configcompiler.RoleStatCalculator
type RoleSkillProgression = configcompiler.RoleSkillProgression
type BattleStrategies = engine.Strategies
type TargetSelector = engine.TargetSelector
type TargetPolicy = targetselector.Selector
type TargetPolicyFunc = targetselector.SelectorFunc
type TargetRegistry = targetselector.Registry
type TargetQuery = engine.TargetQuery
type BattleUnitView = engine.UnitView
type EffectResolver = engine.EffectResolver

// EffectKind 表示技能效果执行的操作类型。
type EffectKind = effectkind.Kind
type EffectHandler = effectresolver.Handler
type EffectHandlerFunc = effectresolver.HandlerFunc
type EffectHandlerFuncs = effectresolver.HandlerFuncs
type EffectRegistry = effectresolver.Registry
type EffectDefinition = effectresolver.Definition
type EffectDefinitions = effectresolver.Definitions
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

// StatusKind 表示已施加状态的行为类型。
type StatusKind = statuskind.Kind
type StatusRegistry = statushandler.Registry
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
func NewDefaultConfigCompiler() *ConfigCompiler { return configcompiler.NewDefault() }

// NewConfigCompiler 创建使用指定战斗模式的配置编译器。
func NewConfigCompiler(modes map[pb.BattleType]BattleMode) *ConfigCompiler {
	return configcompiler.New(mode.Registry(modes))
}

// NewConfigCompilerWithRoleStats 创建带有自定义角色属性计算器的配置编译器。
func NewConfigCompilerWithRoleStats(modes map[pb.BattleType]BattleMode, calculator RoleStatCalculator) *ConfigCompiler {
	return configcompiler.NewWithRoleStats(mode.Registry(modes), calculator)
}

// NewConfigCompilerWithProgression 创建带有自定义属性和技能成长规则的配置编译器。
func NewConfigCompilerWithProgression(modes map[pb.BattleType]BattleMode, roleStats RoleStatCalculator, skills RoleSkillProgression) *ConfigCompiler {
	return configcompiler.NewWithProgression(mode.Registry(modes), roleStats, skills)
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

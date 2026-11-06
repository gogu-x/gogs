package skilleffect

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect/damage"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect/heal"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect/iceburst"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect/status"
	"github.com/gogu-x/gogs/battle/battle/internal/skilleffect/thunder"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Handler 定义单个效果的结算和应用方法。
type Handler interface {
	Resolve(engine.EffectContext) (engine.EffectResolution, error)
	Apply(engine.EffectApplyContext, engine.EffectResolution) error
}

// HandlerFunc 将结算函数适配为效果处理器。
type HandlerFunc func(engine.EffectContext) (engine.EffectResolution, error)

func (f HandlerFunc) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	return f(context)
}

// HandlerFuncs 使用两个函数实现效果处理器。
type HandlerFuncs struct {
	ResolveFunc func(engine.EffectContext) (engine.EffectResolution, error)    // 效果结算函数。
	ApplyFunc   func(engine.EffectApplyContext, engine.EffectResolution) error // 效果应用函数。
}

func (f HandlerFuncs) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	if f.ResolveFunc == nil {
		return engine.EffectResolution{}, fmt.Errorf("effect resolve function is not configured")
	}
	return f.ResolveFunc(context)
}

func (f HandlerFuncs) Apply(context engine.EffectApplyContext, result engine.EffectResolution) error {
	if f.ApplyFunc == nil {
		return fmt.Errorf("effect apply function is not configured")
	}
	return f.ApplyFunc(context, result)
}

// Registry 根据效果类型分发处理器，避免在引擎中添加效果分支。
type Registry map[pb.EffectType]Handler

func (r Registry) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	handler := r[context.Effect.Kind]
	if handler == nil {
		return engine.EffectResolution{}, fmt.Errorf("effect handler is not registered: kind=%d", context.Effect.Kind)
	}
	return handler.Resolve(context)
}

func (r Registry) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	handler := r[resolution.Kind]
	if handler == nil {
		return fmt.Errorf("effect handler is not registered: kind=%d", resolution.Kind)
	}
	return handler.Apply(context, resolution)
}

// Definition 描述配置效果的运行时类型和编译方式。
type Definition struct {
	Kind           pb.EffectType // 运行时效果类型。
	RequiresStatus bool          // 是否需要预先编译状态配置。
	Charge         bool          // 是否为命中后积攒效果。
	Triggers       bool          // 积攒满值时当前技能是否触发效果。
}

// Definitions 保存配置效果类型到运行时定义的映射。
type Definitions map[string]Definition

// DefaultDefinitions 返回内置效果的配置定义。
func DefaultDefinitions() Definitions {
	return Definitions{
		"battle_damage":         {Kind: pb.EffectType_EFFECT_TYPE_DAMAGE},
		"battle_heal":           {Kind: pb.EffectType_EFFECT_TYPE_HEAL},
		"battle_status":         {Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS, RequiresStatus: true},
		"battle_ice_charge":     {Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Charge: true},
		"battle_ice_burst":      {Kind: pb.EffectType_EFFECT_TYPE_ICE_BURST, Charge: true, Triggers: true},
		"battle_thunder_charge": {Kind: pb.EffectType_EFFECT_TYPE_THUNDER, Charge: true},
		"battle_thunder":        {Kind: pb.EffectType_EFFECT_TYPE_THUNDER, Charge: true, Triggers: true},
	}
}

// Default 返回内置效果的处理器注册表。
func Default() Registry {
	return Registry{
		pb.EffectType_EFFECT_TYPE_DAMAGE:       damage.Handler{},
		pb.EffectType_EFFECT_TYPE_HEAL:         heal.Handler{},
		pb.EffectType_EFFECT_TYPE_APPLY_STATUS: status.Handler{},
		pb.EffectType_EFFECT_TYPE_ICE_BURST:    iceburst.Handler{},
		pb.EffectType_EFFECT_TYPE_THUNDER:      thunder.Handler{},
	}
}

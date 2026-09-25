package effectresolver

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
)

type Handler interface {
	Resolve(engine.EffectContext) (engine.EffectResolution, error)
	Apply(engine.EffectApplyContext, engine.EffectResolution) error
}

type HandlerFunc func(engine.EffectContext) (engine.EffectResolution, error)

func (f HandlerFunc) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	return f(context)
}

type HandlerFuncs struct {
	ResolveFunc func(engine.EffectContext) (engine.EffectResolution, error)
	ApplyFunc   func(engine.EffectApplyContext, engine.EffectResolution) error
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

// Registry dispatches effects without putting effect-specific cases in Engine.
type Registry map[effectkind.Kind]Handler

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

type Definition struct {
	Kind           effectkind.Kind
	RequiresStatus bool
}

type Definitions map[string]Definition

func DefaultDefinitions() Definitions {
	return Definitions{
		"battle_damage": {Kind: effectkind.Damage},
		"battle_heal":   {Kind: effectkind.Heal},
		"battle_status": {Kind: effectkind.ApplyStatus, RequiresStatus: true},
	}
}

func Default() Registry {
	return Registry{effectkind.Damage: Damage{}, effectkind.Heal: Heal{}, effectkind.ApplyStatus: ApplyStatus{}}
}

package effectresolver

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type Damage struct{}

func (Damage) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	if context.Damage == nil {
		return engine.EffectResolution{}, fmt.Errorf("damage formula is not configured")
	}
	result := context.Damage.Calculate(engine.DamageInput{
		Attack: context.Actor.Attack, Defense: context.Target.Defense,
		CoefficientPermille: context.Effect.CoefficientPermille, Flat: context.Effect.Flat,
		VariancePermille:       context.Rules.DamageVariancePermille,
		CritChancePermille:     context.Rules.CritChancePermille,
		CritMultiplierPermille: context.Rules.CritMultiplierPermille,
	}, context.RNG)
	detail := ""
	if result.Critical {
		detail = "critical"
	}
	return engine.EffectResolution{Kind: effectkind.Damage, Amount: result.Amount, Critical: result.Critical, Detail: detail}, nil
}

func (Damage) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	before := context.Target.HP()
	amount := resolution.Amount
	if amount > before {
		amount = before
	}
	if amount < 0 {
		amount = 0
	}
	context.Target.SetHP(before - amount)
	context.Emit(pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, "", amount, before, context.Target.HP(), resolution.Detail)
	return nil
}

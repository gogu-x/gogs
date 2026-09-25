package effectresolver

import (
	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type Heal struct{}

func (Heal) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	amount := context.Actor.Attack*context.Effect.CoefficientPermille/1000 + context.Effect.Flat
	if amount < 0 {
		amount = 0
	}
	return engine.EffectResolution{Kind: effectkind.Heal, Amount: amount}, nil
}

func (Heal) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	before := context.Target.HP()
	after := before + resolution.Amount
	if after > context.Target.MaxHP() {
		after = context.Target.MaxHP()
	}
	context.Target.SetHP(after)
	context.Emit(pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, "", after-before, before, after, resolution.Detail)
	return nil
}

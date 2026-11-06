package heal

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Handler 处理治疗效果。
type Handler struct{}

// Resolve 计算治疗量。
func (Handler) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	amount := context.Actor.Attack*context.Effect.CoefficientPermille/1000 + context.Effect.Flat
	if amount < 0 {
		amount = 0
	}
	return engine.EffectResolution{Kind: pb.EffectType_EFFECT_TYPE_HEAL, Amount: amount}, nil
}

// Apply 增加目标生命值并写入治疗事件。
func (Handler) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	before := context.Target.HP()
	after := before + resolution.Amount
	if after > context.Target.MaxHP() {
		after = context.Target.MaxHP()
	}
	context.Target.SetHP(after)
	context.Emit(
		pb.BattleEventType_BATTLE_EVENT_TYPE_HEAL, "", after-before,
		before, after, resolution,
	)
	return nil
}

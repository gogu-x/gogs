package damage

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Handler 处理普通伤害效果。
type Handler struct{}

// Resolve 计算普通伤害。
func (Handler) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	result, err := engine.CalculateDamage(context)
	if err != nil {
		return engine.EffectResolution{}, err
	}
	return engine.EffectResolution{Kind: pb.EffectType_EFFECT_TYPE_DAMAGE, Amount: result.Amount, Critical: result.Critical}, nil
}

// Apply 执行普通伤害。
func (Handler) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	return engine.ApplyDamage(context, resolution)
}

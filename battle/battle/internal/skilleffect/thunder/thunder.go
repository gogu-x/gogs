package thunder

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Handler 处理天雷对单个目标的效果。
type Handler struct{}

// Resolve 计算天雷伤害，并保留天雷自己的事件标识。
func (Handler) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	damage, err := engine.CalculateDamage(context)
	if err != nil {
		return engine.EffectResolution{}, err
	}
	return engine.EffectResolution{
		Kind: pb.EffectType_EFFECT_TYPE_THUNDER, Amount: damage.Amount, Critical: damage.Critical,
	}, nil
}

// Apply 执行天雷伤害；天雷附加状态等规则可在这里独立扩展。
func (Handler) Apply(context engine.EffectApplyContext, result engine.EffectResolution) error {
	return engine.ApplyDamage(context, result)
}

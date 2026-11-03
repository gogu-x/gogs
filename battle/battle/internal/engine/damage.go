package engine

import (
	"fmt"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// DefaultDamageFormula 按攻防、系数、浮动、暴击的顺序计算默认伤害。
type DefaultDamageFormula struct{}

// Calculate 返回默认公式计算出的伤害和暴击结果。
func (DefaultDamageFormula) Calculate(input DamageInput, rng *RNG) DamageResult {
	amount := input.Attack - input.Defense
	if amount < 1 {
		amount = 1
	}
	amount = amount*input.CoefficientPermille/1000 + input.Flat
	if input.VariancePermille > 0 {
		span := input.VariancePermille*2 + 1
		offset := int64(rng.Uint64()%uint64(span)) - input.VariancePermille
		amount = amount * (1000 + offset) / 1000
	}
	critical := input.CritChancePermille > 0 && rng.Permille() < input.CritChancePermille
	if critical {
		amount = amount * input.CritMultiplierPermille / 1000
	}
	if amount < 1 {
		amount = 1
	}
	return DamageResult{Amount: amount, Critical: critical}
}

// CalculateDamage 用已配置公式计算单个目标的基础伤害。
func CalculateDamage(context EffectContext) (DamageResult, error) {
	if context.Damage == nil {
		return DamageResult{}, fmt.Errorf("damage formula is not configured")
	}
	return context.Damage.Calculate(DamageInput{
		Attack: context.Actor.Attack, Defense: context.Target.Defense,
		CoefficientPermille: context.Effect.CoefficientPermille, Flat: context.Effect.Flat,
		VariancePermille:       context.Rules.DamageVariancePermille,
		CritChancePermille:     context.Rules.CritChancePermille,
		CritMultiplierPermille: context.Rules.CritMultiplierPermille,
	}, context.RNG), nil
}

// ApplyDamage 扣除目标生命值并写入伤害事件。
func ApplyDamage(context EffectApplyContext, resolution EffectResolution) error {
	before := context.Target.HP()
	amount := resolution.Amount
	if amount > before {
		amount = before
	}
	if amount < 0 {
		amount = 0
	}
	context.Target.SetHP(before - amount)
	context.Emit(
		pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE, "", amount,
		before, context.Target.HP(), resolution,
	)
	return nil
}

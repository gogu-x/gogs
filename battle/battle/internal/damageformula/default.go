package damageformula

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

// Default preserves the established order: base mitigation, coefficient/flat,
// variance, critical roll, then the minimum damage floor.
type Default struct{}

func (Default) Calculate(input engine.DamageInput, rng *engine.RNG) engine.DamageResult {
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
	return engine.DamageResult{Amount: amount, Critical: critical}
}

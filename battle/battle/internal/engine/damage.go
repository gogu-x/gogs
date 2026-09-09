package engine

type DamageContext struct {
	Attack                 int64
	Defense                int64
	CoefficientPermille    int64
	Flat                   int64
	VariancePermille       int64
	CritChancePermille     int64
	CritMultiplierPermille int64
	Amount                 int64
	Critical               bool
}

type DamageStage interface{ Apply(*DamageContext, *RNG) }
type DamageStageFunc func(*DamageContext, *RNG)

func (f DamageStageFunc) Apply(c *DamageContext, r *RNG) { f(c, r) }

type DamagePipeline struct{ Stages []DamageStage }

func NewDamagePipeline() DamagePipeline {
	return DamagePipeline{Stages: []DamageStage{
		DamageStageFunc(func(c *DamageContext, _ *RNG) {
			c.Amount = c.Attack - c.Defense
			if c.Amount < 1 {
				c.Amount = 1
			}
		}),
		DamageStageFunc(func(c *DamageContext, _ *RNG) { c.Amount = c.Amount*c.CoefficientPermille/1000 + c.Flat }),
		DamageStageFunc(func(c *DamageContext, r *RNG) {
			if c.VariancePermille > 0 {
				span := c.VariancePermille*2 + 1
				offset := int64(r.Uint64()%uint64(span)) - c.VariancePermille
				c.Amount = c.Amount * (1000 + offset) / 1000
			}
		}),
		DamageStageFunc(func(c *DamageContext, r *RNG) {
			if c.CritChancePermille > 0 && r.Permille() < c.CritChancePermille {
				c.Critical = true
				c.Amount = c.Amount * c.CritMultiplierPermille / 1000
			}
		}),
		DamageStageFunc(func(c *DamageContext, _ *RNG) {
			if c.Amount < 1 {
				c.Amount = 1
			}
		}),
	}}
}

func (p DamagePipeline) Calculate(c DamageContext, rng *RNG) DamageContext {
	for _, stage := range p.Stages {
		stage.Apply(&c, rng)
	}
	return c
}

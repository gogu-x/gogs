package battlecfg

import "github.com/gogu-x/gogs/battle/iproto"

// DefaultConfig keeps local development and tests self-contained. Production
// can mount immutable versioned JSON through the config loader.
func DefaultConfig() iproto.Config {
	return iproto.Config{
		Version:                "default",
		ATBThreshold:           100,
		MaxActions:             100,
		DamageVariancePermille: 0,
		CritChancePermille:     0,
		CritMultiplierPermille: 1500,
		Units: map[string]iproto.UnitConfig{
			"hero":  {ID: "hero", MaxHP: 120, Attack: 35, Defense: 8, Speed: 20, BasicSkillID: "attack"},
			"enemy": {ID: "enemy", MaxHP: 100, Attack: 25, Defense: 6, Speed: 15, BasicSkillID: "attack"},
		},
		Skills: map[string]iproto.SkillConfig{
			"attack": {ID: "attack", TargetRule: iproto.TargetEnemySingle, Effects: []iproto.EffectConfig{{Kind: iproto.EffectDamage, CoefficientPermille: 1000}}},
		},
		Statuses: map[string]iproto.StatusConfig{},
	}
}

package engine

import (
	. "github.com/gogu-x/gogs/battle/iproto"
	. "github.com/gogu-x/gogs/glconf/battlecfg"
)

func testConfig() Config {
	return Config{
		Version: "v1", ATBThreshold: 1000, MaxActions: 20,
		CritMultiplierPermille: 1500,
		Statuses: map[string]StatusConfig{
			"stun":    {ID: "stun", Kind: StatusStun, DurationTurns: 1},
			"silence": {ID: "silence", Kind: StatusSilence, DurationTurns: 2},
			"dot":     {ID: "dot", Kind: StatusDOT, DurationTurns: 2, Potency: 7},
			"hot":     {ID: "hot", Kind: StatusHOT, DurationTurns: 2, Potency: 6},
			"buff":    {ID: "buff", Kind: StatusAttributeModifier, DurationTurns: 2, Modifier: AttributeModifier{AttackFlat: 5, DefenseFlat: 4, SpeedFlat: 3}},
		},
		Skills: map[string]SkillConfig{
			"basic":  {ID: "basic", TargetRule: TargetEnemySingle, Effects: []EffectConfig{{Kind: EffectDamage, CoefficientPermille: 1000}}},
			"active": {ID: "active", Cooldown: 2, TargetRule: TargetEnemySingle, Effects: []EffectConfig{{Kind: EffectDamage, CoefficientPermille: 1000, Flat: 1}}},
			"heal":   {ID: "heal", Cooldown: 1, TargetRule: TargetAllyLowestHP, Effects: []EffectConfig{{Kind: EffectHeal, CoefficientPermille: 1000}}},
		},
		Units: map[string]UnitConfig{
			"fighter": {ID: "fighter", MaxHP: 100, Attack: 30, Defense: 10, Speed: 100, BasicSkillID: "basic", ActiveSkillIDs: []string{"active"}},
		},
	}
}

func testInput() BattleInput {
	return BattleInput{ConfigVersion: "v1", Seed: 42, Combatants: []CombatantInput{
		{InstanceID: "a", ConfigID: "fighter", Team: TeamAttacker, Position: 0},
		{InstanceID: "d", ConfigID: "fighter", Team: TeamDefender, Position: 0},
	}}
}

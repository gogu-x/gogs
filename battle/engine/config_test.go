package engine

import (
	"reflect"
	"testing"
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

func TestConfigAndInputValidationTable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config, *BattleInput)
	}{
		{"valid", func(*Config, *BattleInput) {}},
		{"missing version", func(c *Config, _ *BattleInput) { c.Version = "" }},
		{"bad threshold", func(c *Config, _ *BattleInput) { c.ATBThreshold = 0 }},
		{"unknown effect", func(c *Config, _ *BattleInput) { s := c.Skills["basic"]; s.Effects[0].Kind = 99; c.Skills["basic"] = s }},
		{"duplicate id", func(_ *Config, in *BattleInput) { in.Combatants[1].InstanceID = "a" }},
		{"duplicate team position", func(_ *Config, in *BattleInput) {
			in.Combatants = append(in.Combatants, CombatantInput{InstanceID: "a2", ConfigID: "fighter", Team: TeamAttacker, Position: 0})
		}},
		{"unknown unit", func(_ *Config, in *BattleInput) { in.Combatants[0].ConfigID = "missing" }},
		{"one team", func(_ *Config, in *BattleInput) { in.Combatants[1].Team = TeamAttacker; in.Combatants[1].Position = 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, in := testConfig(), testInput()
			tt.mutate(&c, &in)
			err := c.Validate()
			if err == nil {
				err = in.Validate(c)
			}
			if (tt.name == "valid") != (err == nil) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestStableHashesTable(t *testing.T) {
	base := testInput()
	reordered := testInput()
	reordered.Combatants[0], reordered.Combatants[1] = reordered.Combatants[1], reordered.Combatants[0]
	changed := testInput()
	changed.Seed++
	tests := []struct {
		name        string
		left, right BattleInput
		equal       bool
	}{{"order independent", base, reordered, true}, {"seed participates", base, changed, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := StableInputHash(tt.left)
			b, _ := StableInputHash(tt.right)
			if (a == b) != tt.equal {
				t.Fatalf("hash equality=%v", a == b)
			}
		})
	}
	c1, c2 := testConfig(), testConfig()
	delete(c2.Skills, "heal")
	c2.Skills["heal"] = c1.Skills["heal"]
	h1, _ := StableConfigHash(c1)
	h2, _ := StableConfigHash(c2)
	if h1 != h2 {
		t.Fatal("map insertion order changed config hash")
	}
}

func TestMemoryConfigRepositoryTable(t *testing.T) {
	tests := []struct {
		name    string
		run     func(*MemoryConfigRepository) error
		wantErr bool
	}{
		{"put and get isolated copy", func(r *MemoryConfigRepository) error {
			c := testConfig()
			if err := r.Put(c); err != nil {
				return err
			}
			got, err := r.Get("v1")
			if err != nil {
				return err
			}
			got.Units["fighter"] = UnitConfig{}
			again, _ := r.Get("v1")
			if again.Units["fighter"].MaxHP != 100 {
				t.Fatal("repository leaked mutable state")
			}
			return nil
		}, false},
		{"idempotent same version", func(r *MemoryConfigRepository) error {
			if err := r.Put(testConfig()); err != nil {
				return err
			}
			return r.Put(testConfig())
		}, false},
		{"reject conflicting version", func(r *MemoryConfigRepository) error {
			if err := r.Put(testConfig()); err != nil {
				return err
			}
			c := testConfig()
			u := c.Units["fighter"]
			u.MaxHP++
			c.Units["fighter"] = u
			return r.Put(c)
		}, true},
		{"missing version", func(r *MemoryConfigRepository) error { _, err := r.Get("missing"); return err }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewMemoryConfigRepository())
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestFixedRNGTable(t *testing.T) {
	tests := []struct {
		seed uint64
		want []uint64
	}{{0, []uint64{0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f}}, {1, []uint64{0x910a2dec89025cc1, 0xbeeb8da1658eec67, 0xf893a2eefb32555e}}}
	for _, tt := range tests {
		r := NewRNG(tt.seed)
		got := make([]uint64, len(tt.want))
		for i := range got {
			got[i] = r.Uint64()
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("seed %d: got %#x want %#x", tt.seed, got, tt.want)
		}
	}
}

func TestDamagePipelineTable(t *testing.T) {
	tests := []struct {
		name     string
		ctx      DamageContext
		want     int64
		critical bool
	}{
		{"attack defense", DamageContext{Attack: 30, Defense: 10, CoefficientPermille: 1000, CritMultiplierPermille: 1500}, 20, false},
		{"minimum damage", DamageContext{Attack: 5, Defense: 20, CoefficientPermille: 1000, CritMultiplierPermille: 1500}, 1, false},
		{"coefficient and flat", DamageContext{Attack: 30, Defense: 10, CoefficientPermille: 500, Flat: 3, CritMultiplierPermille: 1500}, 13, false},
		{"guaranteed critical", DamageContext{Attack: 30, Defense: 10, CoefficientPermille: 1000, CritChancePermille: 1000, CritMultiplierPermille: 1500}, 30, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDamagePipeline().Calculate(tt.ctx, NewRNG(7))
			if got.Amount != tt.want || got.Critical != tt.critical {
				t.Fatalf("got amount=%d critical=%v", got.Amount, got.Critical)
			}
		})
	}
}

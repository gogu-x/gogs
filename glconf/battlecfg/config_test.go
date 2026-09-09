package battlecfg

import (
	"reflect"
	"testing"

	"github.com/gogu-x/gogs/battle/iproto"
)

func testConfig() iproto.Config {
	return iproto.Config{
		Version: "v1", ATBThreshold: 1000, MaxActions: 20,
		CritMultiplierPermille: 1500,
		Statuses: map[string]iproto.StatusConfig{
			"stun":    {ID: "stun", Kind: iproto.StatusStun, DurationTurns: 1},
			"silence": {ID: "silence", Kind: iproto.StatusSilence, DurationTurns: 2},
			"dot":     {ID: "dot", Kind: iproto.StatusDOT, DurationTurns: 2, Potency: 7},
			"hot":     {ID: "hot", Kind: iproto.StatusHOT, DurationTurns: 2, Potency: 6},
			"buff":    {ID: "buff", Kind: iproto.StatusAttributeModifier, DurationTurns: 2, Modifier: iproto.AttributeModifier{AttackFlat: 5, DefenseFlat: 4, SpeedFlat: 3}},
		},
		Skills: map[string]iproto.SkillConfig{
			"basic":  {ID: "basic", TargetRule: iproto.TargetEnemySingle, Effects: []iproto.EffectConfig{{Kind: iproto.EffectDamage, CoefficientPermille: 1000}}},
			"active": {ID: "active", Cooldown: 2, TargetRule: iproto.TargetEnemySingle, Effects: []iproto.EffectConfig{{Kind: iproto.EffectDamage, CoefficientPermille: 1000, Flat: 1}}},
			"heal":   {ID: "heal", Cooldown: 1, TargetRule: iproto.TargetAllyLowestHP, Effects: []iproto.EffectConfig{{Kind: iproto.EffectHeal, CoefficientPermille: 1000}}},
		},
		Units: map[string]iproto.UnitConfig{
			"fighter": {ID: "fighter", MaxHP: 100, Attack: 30, Defense: 10, Speed: 100, BasicSkillID: "basic", ActiveSkillIDs: []string{"active"}},
		},
	}
}

func testInput() iproto.BattleInput {
	return iproto.BattleInput{ConfigVersion: "v1", Seed: 42, Combatants: []iproto.CombatantInput{
		{InstanceID: "a", ConfigID: "fighter", Team: iproto.TeamAttacker, Position: 0},
		{InstanceID: "d", ConfigID: "fighter", Team: iproto.TeamDefender, Position: 0},
	}}
}

func TestConfigAndInputValidationTable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*iproto.Config, *iproto.BattleInput)
	}{
		{"valid", func(*iproto.Config, *iproto.BattleInput) {}},
		{"missing version", func(c *iproto.Config, _ *iproto.BattleInput) { c.Version = "" }},
		{"bad threshold", func(c *iproto.Config, _ *iproto.BattleInput) { c.ATBThreshold = 0 }},
		{"unknown effect", func(c *iproto.Config, _ *iproto.BattleInput) {
			s := c.Skills["basic"]
			s.Effects[0].Kind = 99
			c.Skills["basic"] = s
		}},
		{"duplicate id", func(_ *iproto.Config, in *iproto.BattleInput) { in.Combatants[1].InstanceID = "a" }},
		{"duplicate team position", func(_ *iproto.Config, in *iproto.BattleInput) {
			in.Combatants = append(in.Combatants, iproto.CombatantInput{InstanceID: "a2", ConfigID: "fighter", Team: iproto.TeamAttacker, Position: 0})
		}},
		{"unknown unit", func(_ *iproto.Config, in *iproto.BattleInput) { in.Combatants[0].ConfigID = "missing" }},
		{"one team", func(_ *iproto.Config, in *iproto.BattleInput) {
			in.Combatants[1].Team = iproto.TeamAttacker
			in.Combatants[1].Position = 1
		}},
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
		left, right iproto.BattleInput
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
			got.Units["fighter"] = iproto.UnitConfig{}
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

func TestLoadDefaultConfigRepository(t *testing.T) {
	repo, err := LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if c.Units["hero"].MaxHP != 120 || c.Skills["attack"].TargetRule != iproto.TargetEnemySingle {
		t.Fatalf("unexpected default config: %+v", c)
	}
}

func TestBattleInputCanonicalOrdering(t *testing.T) {
	a := testInput()
	b := testInput()
	b.Combatants[0], b.Combatants[1] = b.Combatants[1], b.Combatants[0]
	ca, cb := CanonicalInput(a), CanonicalInput(b)
	if !reflect.DeepEqual(ca, cb) {
		t.Fatalf("canonical inputs differ: %+v vs %+v", ca, cb)
	}
}

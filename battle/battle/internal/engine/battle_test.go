package engine

import (
	"reflect"
	"testing"

	. "github.com/gogu-x/gogs/battle/iproto"
	. "github.com/gogu-x/gogs/glconf/battlecfg"
)

func newTestBattle(t *testing.T, c Config, in BattleInput) *Battle {
	t.Helper()
	r := NewMemoryConfigRepository()
	if err := r.Put(c); err != nil {
		t.Fatal(err)
	}
	b, err := NewBattle(r, in)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func findUnit(b *Battle, id string) *unit {
	for _, u := range b.units {
		if u.Input.InstanceID == id {
			return u
		}
	}
	return nil
}
func actionActors(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Type == EventActionStarted {
			out = append(out, e.ActorID)
		}
	}
	return out
}
func skillEvents(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Type == EventSkillUsed {
			out = append(out, e.SkillID)
		}
	}
	return out
}

func TestIntegerEventJumpATBTable(t *testing.T) {
	tests := []struct {
		name                         string
		attackerSpeed, defenderSpeed int64
		wantActors                   []string
		wantTicks                    []int64
	}{
		{"equal speed stable tie", 100, 100, []string{"a", "d", "a"}, []int64{10, 10, 20}},
		{"faster unit accumulates gauge", 100, 50, []string{"a", "a", "d"}, []int64{10, 20, 20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig()
			c.MaxActions = 3
			c.Skills["active"] = SkillConfig{ID: "active", Cooldown: 99, TargetRule: TargetEnemySingle, Effects: []EffectConfig{{Kind: EffectDamage, CoefficientPermille: 0, Flat: 1}}}
			a := c.Units["fighter"]
			a.Speed = tt.attackerSpeed
			c.Units["attacker"] = func() UnitConfig { a.ID = "attacker"; return a }()
			d := a
			d.ID = "defender"
			d.Speed = tt.defenderSpeed
			c.Units["defender"] = d
			delete(c.Units, "fighter")
			in := testInput()
			in.Combatants[0].ConfigID = "attacker"
			in.Combatants[1].ConfigID = "defender"
			b := newTestBattle(t, c, in)
			res, err := b.Run()
			if err != nil {
				t.Fatal(err)
			}
			if got := actionActors(res.Events); !reflect.DeepEqual(got, tt.wantActors) {
				t.Fatalf("actors=%v", got)
			}
			var ticks []int64
			for _, e := range res.Events {
				if e.Type == EventActionStarted {
					ticks = append(ticks, e.Tick)
				}
			}
			if !reflect.DeepEqual(ticks, tt.wantTicks) {
				t.Fatalf("ticks=%v", ticks)
			}
		})
	}
}

func TestFourTargetRulesTable(t *testing.T) {
	c := testConfig()
	in := testInput()
	in.Combatants = []CombatantInput{
		{InstanceID: "a0", ConfigID: "fighter", Team: TeamAttacker, Position: 0},
		{InstanceID: "a1", ConfigID: "fighter", Team: TeamAttacker, Position: 1},
		{InstanceID: "d0", ConfigID: "fighter", Team: TeamDefender, Position: 0},
		{InstanceID: "d1", ConfigID: "fighter", Team: TeamDefender, Position: 1},
	}
	b := newTestBattle(t, c, in)
	findUnit(b, "a1").HP = 20
	tests := []struct {
		name string
		rule TargetRule
		want []string
	}{{"enemy single", TargetEnemySingle, []string{"d0"}}, {"enemy all", TargetEnemyAll, []string{"d0", "d1"}}, {"ally lowest hp", TargetAllyLowestHP, []string{"a1"}}, {"self", TargetSelf, []string{"a0"}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUnits := b.targets(findUnit(b, "a0"), tt.rule)
			got := make([]string, len(gotUnits))
			for i, u := range gotUnits {
				got[i] = u.Input.InstanceID
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("targets=%v", got)
			}
		})
	}
}

func TestBasicActiveCooldownAndAITable(t *testing.T) {
	tests := []struct {
		name     string
		cooldown int
		status   StatusKind
		want     string
	}{{"active preferred", 0, 0, "active"}, {"basic while active cooling", 2, 0, "basic"}, {"basic while silenced", 0, StatusSilence, "basic"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBattle(t, testConfig(), testInput())
			u := findUnit(b, "a")
			if tt.cooldown > 0 {
				u.Cooldowns["active"] = tt.cooldown
			}
			if tt.status != 0 {
				u.Statuses = []activeStatus{{Config: b.config.Statuses["silence"], Remaining: 2}}
			}
			if got := b.chooseSkill(u).ID; got != tt.want {
				t.Fatalf("skill=%s", got)
			}
		})
	}
	for _, tt := range []struct {
		name string
		cd   int
		want []string
	}{{"cooldown two", 2, []string{"active", "basic", "active"}}, {"cooldown three", 3, []string{"active", "basic", "basic"}}} {
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig()
			s := c.Skills["active"]
			s.Cooldown = tt.cd
			c.Skills["active"] = s
			b := newTestBattle(t, c, testInput())
			u := findUnit(b, "a")
			for range 3 {
				b.execute(u)
			}
			if got := skillEvents(b.events); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("skills=%v", got)
			}
		})
	}
}

func TestEffectChainTable(t *testing.T) {
	tests := []struct {
		name    string
		effect  EffectConfig
		prepare func(*unit)
		check   func(*testing.T, *Battle, *unit)
	}{
		{"damage", EffectConfig{Kind: EffectDamage, CoefficientPermille: 1000}, func(*unit) {}, func(t *testing.T, _ *Battle, u *unit) {
			if u.HP != 80 {
				t.Fatalf("hp=%d", u.HP)
			}
		}},
		{"heal", EffectConfig{Kind: EffectHeal, CoefficientPermille: 1000}, func(u *unit) { u.HP = 50 }, func(t *testing.T, _ *Battle, u *unit) {
			if u.HP != 80 {
				t.Fatalf("hp=%d", u.HP)
			}
		}},
		{"apply status", EffectConfig{Kind: EffectApplyStatus, StatusID: "stun"}, func(*unit) {}, func(t *testing.T, _ *Battle, u *unit) {
			if len(u.Statuses) != 1 || u.Statuses[0].Config.ID != "stun" {
				t.Fatalf("statuses=%v", u.Statuses)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBattle(t, testConfig(), testInput())
			actor, target := findUnit(b, "a"), findUnit(b, "d")
			tt.prepare(target)
			skill := SkillConfig{ID: "test", TargetRule: TargetEnemySingle, Effects: []EffectConfig{tt.effect}}
			b.applyEffect(actor, target, skill, tt.effect)
			tt.check(t, b, target)
		})
	}
	b := newTestBattle(t, testConfig(), testInput())
	target := findUnit(b, "d")
	skill := SkillConfig{ID: "chain", TargetRule: TargetEnemySingle, Effects: []EffectConfig{{Kind: EffectDamage, CoefficientPermille: 1000}, {Kind: EffectApplyStatus, StatusID: "stun"}}}
	for _, e := range skill.Effects {
		b.applyEffect(findUnit(b, "a"), target, skill, e)
	}
	if len(b.events) != 2 || b.events[0].Type != EventDamage || b.events[1].Type != EventStatusApplied {
		t.Fatalf("effect order=%v", b.events)
	}
}

func TestStatusMechanicsTable(t *testing.T) {
	tests := []struct {
		name, status string
		start, want  int64
		event        EventType
	}{{"dot", "dot", 50, 43, EventDamage}, {"hot", "hot", 50, 56, EventHeal}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBattle(t, testConfig(), testInput())
			u := findUnit(b, "a")
			u.HP = tt.start
			u.Statuses = []activeStatus{{Config: b.config.Statuses[tt.status], Remaining: 2}}
			b.tickPeriodic(u)
			if u.HP != tt.want || len(b.events) != 1 || b.events[0].Type != tt.event {
				t.Fatalf("hp=%d events=%v", u.HP, b.events)
			}
		})
	}
	b := newTestBattle(t, testConfig(), testInput())
	u := findUnit(b, "a")
	u.Statuses = []activeStatus{{Config: b.config.Statuses["buff"], Remaining: 2}}
	if u.attack() != 35 || u.defense() != 14 || u.speed() != 103 {
		t.Fatalf("modified attributes=%d/%d/%d", u.attack(), u.defense(), u.speed())
	}
	b = newTestBattle(t, testConfig(), testInput())
	u = findUnit(b, "a")
	u.Statuses = []activeStatus{{Config: b.config.Statuses["stun"], Remaining: 1}}
	b.execute(u)
	types := map[EventType]bool{}
	for _, e := range b.events {
		types[e.Type] = true
	}
	if !types[EventActionSkipped] || types[EventSkillUsed] {
		t.Fatalf("stun events=%v", b.events)
	}
}

func TestActionLevelEventsTable(t *testing.T) {
	tests := []struct {
		name    string
		stunned bool
		want    []EventType
	}{{"normal action", false, []EventType{EventActionStarted, EventSkillUsed, EventDamage, EventActionEnded}}, {"stunned action", true, []EventType{EventActionStarted, EventActionSkipped, EventActionEnded}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBattle(t, testConfig(), testInput())
			u := findUnit(b, "a")
			if tt.stunned {
				u.Statuses = []activeStatus{{Config: b.config.Statuses["stun"], Remaining: 1}}
			}
			b.execute(u)
			got := make([]EventType, len(b.events))
			for i, e := range b.events {
				got[i] = e.Type
				if e.Action != 1 {
					t.Fatalf("event lacks action grouping: %+v", e)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("types=%v", got)
			}
		})
	}
}

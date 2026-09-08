package engine

import "testing"

func TestDeterministicBattleIDAndChecksumTable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BattleInput)
		same   bool
	}{{"same input", func(*BattleInput) {}, true}, {"combatant order canonical", func(in *BattleInput) { in.Combatants[0], in.Combatants[1] = in.Combatants[1], in.Combatants[0] }, true}, {"seed changes identity", func(in *BattleInput) { in.Seed++ }, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewMemoryConfigRepository()
			if err := r.Put(testConfig()); err != nil {
				t.Fatal(err)
			}
			base := testInput()
			other := testInput()
			tt.mutate(&other)
			a, _ := NewBattle(r, base)
			b, _ := NewBattle(r, other)
			ra, _ := a.Run()
			rb, _ := b.Run()
			if (ra.BattleID == rb.BattleID) != tt.same {
				t.Fatalf("ids %s %s", ra.BattleID, rb.BattleID)
			}
			if tt.same && ra.Checksum != rb.Checksum {
				t.Fatalf("checksums differ %s %s", ra.Checksum, rb.Checksum)
			}
		})
	}
}

func TestReplayVerificationTable(t *testing.T) {
	r := NewMemoryConfigRepository()
	if err := r.Put(testConfig()); err != nil {
		t.Fatal(err)
	}
	b, err := NewBattle(r, testInput())
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Run()
	if err != nil {
		t.Fatal(err)
	}
	clone := func() Replay { x := result.Replay; x.Events = append([]Event(nil), x.Events...); return x }
	tests := []struct {
		name    string
		mutate  func(*Replay)
		wantErr bool
	}{{"valid", func(*Replay) {}, false}, {"event tamper", func(r *Replay) { r.Events[0].Tick++ }, true}, {"checksum tamper", func(r *Replay) { r.Checksum = "00" + r.Checksum[2:] }, true}, {"battle id tamper", func(r *Replay) { r.BattleID = "tampered" }, true}, {"config hash tamper", func(r *Replay) { r.ConfigHash = "tampered" }, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replay := clone()
			tt.mutate(&replay)
			err := VerifyReplay(r, replay)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

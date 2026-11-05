package souljade

import (
	"bytes"
	"errors"
	"testing"

	"github.com/gogu-x/gogs/glconf"
)

type failedRandom struct{}

func (failedRandom) Read([]byte) (int, error) { return 0, errors.New("random unavailable") }

func TestRollRefineAffixes(t *testing.T) {
	level := &glconf.SoulJadeLevelCfg{AffixCount: 2, RefineMinPermille: 7, RefineMaxPermille: 7}
	pool := []refineCandidate{{id: 1, weight: 1}, {id: 1001, weight: 1, effect: true}}
	affixes, err := rollRefineAffixes(pool, level, bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if len(affixes) != 2 || affixes[0].GetAttribute() != 1 || affixes[0].GetPermille() != 7 || affixes[1].GetAttribute() != 1001 || affixes[1].GetPermille() != 0 {
		t.Fatalf("unexpected affixes: %+v", affixes)
	}
	if _, err := rollRefineAffixes(pool, level, failedRandom{}); err == nil {
		t.Fatal("random failure was ignored")
	}
}

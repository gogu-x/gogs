package engine

import (
	"reflect"
	"testing"
)

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

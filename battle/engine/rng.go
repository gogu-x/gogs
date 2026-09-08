package engine

// RNG is SplitMix64: all operations are explicitly uint64 and platform independent.
type RNG struct{ state uint64 }

func NewRNG(seed uint64) *RNG { return &RNG{state: seed} }

func (r *RNG) Uint64() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *RNG) Permille() int64 { return int64(r.Uint64() % 1000) }

package engine

// RNG is SplitMix64: all operations are explicitly uint64 and platform independent.
type RNG struct{ state uint64 }

// NewRNG 按指定种子创建可复现的随机数生成器。
func NewRNG(seed uint64) *RNG { return &RNG{state: seed} }

// Uint64 推进随机状态并返回下一个随机数。
func (r *RNG) Uint64() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Permille 生成零到九百九十九之间的千分比判定值。
func (r *RNG) Permille() int64 { return int64(r.Uint64() % 1000) }

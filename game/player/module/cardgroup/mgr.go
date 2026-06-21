package cardgroup

type Mgr struct {
	cards map[uint64]int64
}

func New() *Mgr {
	return &Mgr{cards: make(map[uint64]int64)}
}

func (m *Mgr) Add(id uint64, amount int64) {
	m.cards[id] = amount
}

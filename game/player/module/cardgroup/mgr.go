package cardgroup

type Mgr struct {
	Cards map[uint64]int64 `bson:"cards"`
}

func New() *Mgr {
	return &Mgr{Cards: make(map[uint64]int64)}
}

func (m *Mgr) Add(id uint64, amount int64) {
	m.Cards[id] = amount
}

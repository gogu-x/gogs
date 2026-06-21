package asset

type Mgr struct {
	Assets map[uint64]int64 `bson:"assets"`
}

func (m *Mgr) Add(id uint64, value int64) {
	if m.Assets == nil {
		m.Assets = make(map[uint64]int64)
	}
	m.Assets[id] += value
}

func (m *Mgr) Del(id uint64, value int64) {
	m.Assets[id] -= value
}

func (m *Mgr) Get(id uint64) int64 {
	return m.Assets[id]
}

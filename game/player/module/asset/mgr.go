package asset

type Mgr struct {
	assets map[uint64]int64
}

func (m *Mgr) Add(id uint64, value int64) {
	if m.assets == nil {
		m.assets = make(map[uint64]int64)
	}
	m.assets[id] += value
}

func (m *Mgr) Del(id uint64, value int64) {
	m.assets[id] -= value
}

func (m *Mgr) Get(id uint64) int64 {
	return m.assets[id]
}

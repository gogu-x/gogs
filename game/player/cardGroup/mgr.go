package cardGroup

type Mgr struct {
	cards map[uint64]int64
}

func newCardGroupMgr() *Mgr {
	return &Mgr{cards: make(map[uint64]int64)}
}

func (cm *Mgr) add(id uint64, amount int64) {
	cm.cards[id] = amount
}

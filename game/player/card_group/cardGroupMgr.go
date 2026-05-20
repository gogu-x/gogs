package card_group

type cardGroupMgr struct {
	cards map[uint64]int64
}

func newCardGroupMgr() *cardGroupMgr {
	return &cardGroupMgr{cards: make(map[uint64]int64)}
}

func (cm *cardGroupMgr) add(id uint64, amount int64) {
	cm.cards[id] = amount
}

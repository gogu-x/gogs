package bag

type Item struct {
	ItemID uint64
	Count  int32
}

type BagMgr struct {
	slots []*Item
}

func (b *BagMgr) Add(itemID uint64, count int32) {
	if b.slots == nil {
		b.slots = make([]*Item, 100)
	}
	for _, s := range b.slots {
		if s != nil && s.ItemID == itemID {
			s.Count += count
			return
		}
	}
	for i, s := range b.slots {
		if s == nil {
			b.slots[i] = &Item{ItemID: itemID, Count: count}
			return
		}
	}
}

func (b *BagMgr) Get(itemID uint64) int32 {
	for _, s := range b.slots {
		if s != nil && s.ItemID == itemID {
			return s.Count
		}
	}
	return 0
}

func (b *BagMgr) Remove(itemID uint64, count int32) {
	for i, s := range b.slots {
		if s != nil && s.ItemID == itemID {
			s.Count -= count
			if s.Count <= 0 {
				b.slots[i] = nil
			}
			return
		}
	}
}

package bag

type Item struct {
	ItemID uint64
	Count  int32
}

type Mgr struct {
	slots map[uint64]*Item
}

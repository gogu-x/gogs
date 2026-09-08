package bag

type Item struct {
	ItemID uint64
	Count  int32
}

type Mgr struct {
	Slots map[uint64]*Item `bson:"slots"`
}

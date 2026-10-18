package bag

// +deepcopy-gen=true
type Item struct {
	ItemID uint64
	Count  int32
}

// +deepcopy-gen=true
type Mgr struct {
	Slots map[uint64]*Item `bson:"slots"`
}

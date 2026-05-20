package assest

type AssetsMgr struct {
	assets map[uint64]int64
}

func (am *AssetsMgr) Add(id uint64, value int64) {
	if am.assets == nil {
		am.assets = make(map[uint64]int64)
	}
	am.assets[id] += value
}

func (am *AssetsMgr) Del(id uint64, value int64) {
	am.assets[id] -= value
}

func (am *AssetsMgr) Get(id uint64) int64 {
	return am.assets[id]
}

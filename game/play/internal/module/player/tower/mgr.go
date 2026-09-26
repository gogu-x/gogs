package tower

// +deepcopy-gen=true
type Mgr struct {
	Layer int32 `bson:"layer"`
}

func NewTowerMgr() *Mgr {
	return &Mgr{
		Layer: 1,
	}
}

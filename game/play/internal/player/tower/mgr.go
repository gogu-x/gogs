package tower

// Mgr 保存玩家爬塔模块数据。
// +deepcopy-gen=true
type Mgr struct {
	Layer int32 `bson:"layer"` // 当前爬塔层数。
}

// NewTowerMgr 创建初始层数为 1 的爬塔模块。
func NewTowerMgr() *Mgr {
	return &Mgr{
		Layer: 1,
	}
}

// GetLayer 返回玩家当前爬塔层数。
func (mgr *Mgr) GetLayer() int32 {
	if mgr == nil {
		return 0
	}
	return mgr.Layer
}

// SetLayer 更新玩家当前爬塔层数。
func (mgr *Mgr) SetLayer(layer int32) {
	if mgr != nil {
		mgr.Layer = layer
	}
}

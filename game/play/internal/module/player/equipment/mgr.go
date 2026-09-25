package equipment

// Item 保存装备模块管理的单件装备及其养成数据。
type Item struct {
	ID          string `bson:"id"`
	ConfigID    int32  `bson:"config_id"`
	Level       int32  `bson:"level"`
	RefineLevel int32  `bson:"refine_level"`
}

// Mgr 管理玩家拥有的装备实例。
type Mgr struct {
	Items map[string]*Item `bson:"items"`
}

// New 创建空的装备管理器。
func New() *Mgr { return &Mgr{Items: make(map[string]*Item)} }

// Get 返回指定装备的副本。
func (m *Mgr) Get(id string) (*Item, bool) {
	if m == nil || id == "" {
		return nil, false
	}
	item, ok := m.Items[id]
	if !ok || item == nil || item.ID != id {
		return nil, false
	}
	copyItem := *item
	return &copyItem, true
}

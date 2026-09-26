package equipment

// Item 保存装备模块管理的单件装备及其养成数据。
// +deepcopy-gen=true
type Item struct {
	ID          string `bson:"id"`
	ConfigID    int32  `bson:"config_id"`
	Slot        int32  `bson:"slot"`
	Level       int32  `bson:"level"`
	RefineLevel int32  `bson:"refine_level"`
}

// Mgr 管理玩家拥有的装备实例。
// +deepcopy-gen=true
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

// GetItems 返回装备实例副本列表。
func (m *Mgr) GetItems() []*Item {
	if m == nil {
		return nil
	}
	items := make([]*Item, 0, len(m.Items))
	for _, item := range m.Items {
		if item != nil {
			copyItem := *item
			items = append(items, &copyItem)
		}
	}
	return items
}

// GetID 返回装备实例 ID。
func (item *Item) GetID() string {
	if item == nil {
		return ""
	}
	return item.ID
}

// GetConfigID 返回装备配置 ID。
func (item *Item) GetConfigID() int32 {
	if item == nil {
		return 0
	}
	return item.ConfigID
}

// GetSlot 返回装备槽位。
func (item *Item) GetSlot() int32 {
	if item == nil {
		return 0
	}
	return item.Slot
}

// GetLevel 返回装备等级。
func (item *Item) GetLevel() int32 {
	if item == nil {
		return 0
	}
	return item.Level
}

// GetRefineLevel 返回装备精炼等级。
func (item *Item) GetRefineLevel() int32 {
	if item == nil {
		return 0
	}
	return item.RefineLevel
}

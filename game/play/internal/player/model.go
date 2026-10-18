package player

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// PersistenceModel 提供单个玩家文档的加载、快照和存盘元数据。
type PersistenceModel struct {
	current *Player
}

// NewPersistenceModel 创建绑定指定玩家数据的存盘模型。
func NewPersistenceModel(current *Player) *PersistenceModel {
	return &PersistenceModel{current: current}
}

// SetPlayer 更新模型持有的当前玩家指针。
func (model *PersistenceModel) SetPlayer(current *Player) {
	model.current = current
}

// GetCollection 返回玩家文档集合名。
func (model *PersistenceModel) GetCollection() string { return "player" }

// GetPrototype 返回玩家文档原型。
func (model *PersistenceModel) GetPrototype() interface{} { return (*Player)(nil) }

// GetIDField 返回玩家文档主键字段名。
func (model *PersistenceModel) GetIDField() string { return "_id" }

// GetKey 从玩家文档中提取玩家编号。
func (model *PersistenceModel) GetKey(data interface{}) interface{} {
	current, ok := data.(*Player)
	if !ok || current == nil {
		return nil
	}
	return current.GetUID()
}

// Clone 创建可异步存盘的玩家快照。
func (model *PersistenceModel) Clone(data interface{}) interface{} {
	current, ok := data.(*Player)
	if !ok || current == nil {
		return nil
	}
	snapshot := current.DeepCopy()
	snapshot.SetDirty(false)
	return snapshot
}

// Range 遍历模型绑定的当前玩家。
func (model *PersistenceModel) Range(callback func(interface{}) bool) {
	if model.current != nil {
		callback(model.current)
	}
}

// GetSaveInterval 返回 0，表示存盘定时器由玩家角色自行管理。
func (model *PersistenceModel) GetSaveInterval() time.Duration { return 0 }

// AfterSave 在快照版本仍为最新时清除当前玩家的脏标记。
func (model *PersistenceModel) AfterSave(_ tree.Context, data interface{}) error {
	snapshot, ok := data.(*Player)
	current := model.current
	if !ok || snapshot == nil || current == nil {
		return nil
	}
	if current.Revision() == snapshot.Revision() {
		current.SetDirty(false)
	}
	return nil
}

// MackDirty 返回玩家是否需要存盘。
func (model *PersistenceModel) MackDirty(data interface{}) bool {
	current, ok := data.(*Player)
	return ok && current != nil && current.IsDirty()
}

// GetTickerType 返回该模型的定时器类型；自动存盘间隔为 0 时不会注册定时任务。
func (model *PersistenceModel) GetTickerType() timer.TimerType {
	return timer.TimerType(1000001)
}

package store

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// PersistenceModel 将内存中的联盟聚合适配到 Mongo 存盘流程。
type PersistenceModel struct {
	store      *Store
	tickerType timer.TimerType
}

// NewPersistenceModel 绑定联盟仓库及 Actor 使用的存盘定时器类型。
func NewPersistenceModel(store *Store, tickerType timer.TimerType) *PersistenceModel {
	return &PersistenceModel{store: store, tickerType: tickerType}
}

// GetCollection 返回联盟文档集合名。
func (model *PersistenceModel) GetCollection() string { return "guild" }

// GetPrototype 返回联盟文档原型。
func (model *PersistenceModel) GetPrototype() interface{} { return (*Guild)(nil) }

// GetIDField 返回联盟文档主键字段。
func (model *PersistenceModel) GetIDField() string { return "_id" }

// GetKey 从联盟文档提取主键。
func (model *PersistenceModel) GetKey(data interface{}) interface{} {
	guild, ok := data.(*Guild)
	if !ok || guild == nil {
		return nil
	}
	return guild.GetID()
}

// Clone 创建独立的联盟存盘快照。
func (model *PersistenceModel) Clone(data interface{}) interface{} {
	guild, ok := data.(*Guild)
	if !ok || guild == nil {
		return nil
	}
	snapshot := guild.DeepCopy()
	snapshot.SetDirty(false)
	return snapshot
}

// Range 遍历待存盘的联盟文档。
func (model *PersistenceModel) Range(callback func(interface{}) bool) {
	if model == nil || model.store == nil || callback == nil {
		return
	}
	model.store.Range(func(guild *Guild) bool {
		if guild == nil || !guild.IsDirty() {
			return true
		}
		return callback(guild)
	})
}

// GetSaveInterval 返回定期存盘间隔。
func (model *PersistenceModel) GetSaveInterval() time.Duration { return time.Minute }

// AfterSave 在快照版本仍为最新时清除脏标记。
func (model *PersistenceModel) AfterSave(_ tree.Context, data interface{}) error {
	snapshot, ok := data.(*Guild)
	if !ok || snapshot == nil || model == nil || model.store == nil {
		return nil
	}
	current := model.store.getGuild(snapshot.GetID())
	if current != nil && current.Revision() == snapshot.Revision() {
		current.SetDirty(false)
	}
	return nil
}

// MackDirty 返回联盟文档是否需要存盘。
func (model *PersistenceModel) MackDirty(data interface{}) bool {
	guild, ok := data.(*Guild)
	return ok && guild != nil && guild.IsDirty()
}

// GetTickerType 返回 Actor 定期存盘事件的类型。
func (model *PersistenceModel) GetTickerType() timer.TimerType { return model.tickerType }

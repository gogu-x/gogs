package base

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Comp 是单条数据组件：一个活动实例一份，Data 由模板直接读写，修改后调用 MarkDirty。
type Comp[T any] struct {
	id       int32
	Data     T
	dirty    bool
	revision uint64
}

// NewComp 创建指定编号的单条数据组件。
func NewComp[T any](id int32) *Comp[T] { return &Comp[T]{id: id} }

// CompID 返回组件编号。
func (c *Comp[T]) CompID() int32 { return c.id }

// MarkDirty 标记组件数据需要存盘。
func (c *Comp[T]) MarkDirty() {
	c.dirty = true
	c.revision++
}

// IsDirty 返回组件数据是否需要存盘。
func (c *Comp[T]) IsDirty() bool { return c.dirty }

// Revision 返回组件数据的内存版本号。
func (c *Comp[T]) Revision() uint64 { return c.revision }

// ClearDirty 在存盘快照版本与当前版本一致时清除脏标记。
func (c *Comp[T]) ClearDirty(revision uint64) {
	if c.revision == revision {
		c.dirty = false
	}
}

// Load 从存盘数据恢复组件，空数据保留默认值。
func (c *Comp[T]) Load(raw bson.Raw) error {
	if len(raw) == 0 {
		return nil
	}
	return bson.Unmarshal(raw, &c.Data)
}

// Save 生成组件数据的存盘快照。
func (c *Comp[T]) Save() (bson.Raw, error) {
	raw, err := bson.Marshal(&c.Data)
	return bson.Raw(raw), err
}

// PlayerComp 是玩家列表组件：每个玩家一份条目，随玩家上线加载、释放时卸载。
type PlayerComp[T any] struct {
	id       int32
	loaded   map[uint64]bool
	players  map[uint64]*T
	dirty    map[uint64]bool
	revision map[uint64]uint64
}

// NewPlayerComp 创建指定编号的玩家列表组件。
func NewPlayerComp[T any](id int32) *PlayerComp[T] {
	return &PlayerComp[T]{
		id:       id,
		loaded:   make(map[uint64]bool),
		players:  make(map[uint64]*T),
		dirty:    make(map[uint64]bool),
		revision: make(map[uint64]uint64),
	}
}

// CompID 返回组件编号。
func (c *PlayerComp[T]) CompID() int32 { return c.id }

// Get 返回已加载玩家的条目，条目不存在时新建；玩家未加载时返回 nil，避免覆盖库中数据。
func (c *PlayerComp[T]) Get(uid uint64) *T {
	if !c.loaded[uid] {
		return nil
	}
	data := c.players[uid]
	if data == nil {
		data = new(T)
		c.players[uid] = data
	}
	return data
}

// MarkDirty 标记指定玩家的条目需要存盘；玩家未加载时忽略。
func (c *PlayerComp[T]) MarkDirty(uid uint64) {
	if c.players[uid] == nil {
		return
	}
	c.dirty[uid] = true
	c.revision[uid]++
}

// Range 遍历已存在的玩家条目，回调返回 false 时停止。
func (c *PlayerComp[T]) Range(fn func(uid uint64, data *T) bool) {
	for uid, data := range c.players {
		if !fn(uid, data) {
			return
		}
	}
}

// Load 从存盘数据恢复指定玩家的条目。
func (c *PlayerComp[T]) Load(uid uint64, raw bson.Raw) error {
	data := new(T)
	if len(raw) > 0 {
		if err := bson.Unmarshal(raw, data); err != nil {
			return err
		}
	}
	c.loaded[uid] = true
	c.players[uid] = data
	return nil
}

// Save 生成指定玩家条目的存盘快照。
func (c *PlayerComp[T]) Save(uid uint64) (bson.Raw, error) {
	raw, err := bson.Marshal(c.players[uid])
	return bson.Raw(raw), err
}

// SetLoaded 标记玩家数据已加载。
func (c *PlayerComp[T]) SetLoaded(uid uint64) { c.loaded[uid] = true }

// Unload 卸载指定玩家的条目。
func (c *PlayerComp[T]) Unload(uid uint64) {
	delete(c.loaded, uid)
	delete(c.players, uid)
	delete(c.dirty, uid)
	delete(c.revision, uid)
}

// DirtyPlayers 返回有待存盘条目的玩家。
func (c *PlayerComp[T]) DirtyPlayers() []uint64 {
	uids := make([]uint64, 0, len(c.dirty))
	for uid, dirty := range c.dirty {
		if dirty && c.players[uid] != nil {
			uids = append(uids, uid)
		}
	}
	return uids
}

// IsDirty 返回指定玩家条目是否需要存盘。
func (c *PlayerComp[T]) IsDirty(uid uint64) bool { return c.dirty[uid] && c.players[uid] != nil }

// Revision 返回指定玩家条目的内存版本号。
func (c *PlayerComp[T]) Revision(uid uint64) uint64 { return c.revision[uid] }

// ClearDirty 在存盘快照版本与当前版本一致时清除脏标记。
func (c *PlayerComp[T]) ClearDirty(uid uint64, revision uint64) {
	if c.dirty[uid] && c.revision[uid] == revision {
		delete(c.dirty, uid)
	}
}

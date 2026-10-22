package base

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Initializer 由模板数据结构可选实现（指针接收者），用于补齐 map/slice 等运行时容器和固定默认值。
// 组件在数据新建和从库中恢复后都会调用，因此实现必须幂等：只补齐为空的字段，不覆盖已有值。
type Initializer interface {
	Init()
}

// initData 对实现了 Initializer 的数据调用 Init。
func initData[T any](data *T) {
	if initializer, ok := any(data).(Initializer); ok {
		initializer.Init()
	}
}

// Comp 是单条数据组件：一个活动实例一份，Data 由模板直接读写，修改后调用 MarkDirty。
// 存盘数据解码失败时组件进入阻断状态，不再存盘，避免默认值覆盖库中原数据。
type Comp[T any] struct {
	id       int32
	Data     T
	dirty    bool
	revision uint64
	blocked  bool
}

// NewComp 创建指定编号的单条数据组件，并初始化数据。
func NewComp[T any](id int32) *Comp[T] {
	comp := &Comp[T]{id: id}
	initData(&comp.Data)
	return comp
}

// CompID 返回组件编号。
func (c *Comp[T]) CompID() int32 { return c.id }

// MarkDirty 标记组件数据需要存盘；阻断状态下忽略。
func (c *Comp[T]) MarkDirty() {
	if c.blocked {
		return
	}
	c.dirty = true
	c.revision++
}

// IsDirty 返回组件数据是否需要存盘。
func (c *Comp[T]) IsDirty() bool { return c.dirty && !c.blocked }

// Blocked 返回组件是否因存盘数据解码失败被阻断。
func (c *Comp[T]) Blocked() bool { return c.blocked }

// Revision 返回组件数据的内存版本号。
func (c *Comp[T]) Revision() uint64 { return c.revision }

// ClearDirty 在存盘快照版本与当前版本一致时清除脏标记。
func (c *Comp[T]) ClearDirty(revision uint64) {
	if c.revision == revision {
		c.dirty = false
	}
}

// Load 从存盘数据恢复组件，空数据保留默认值；解码成功后调用 Init 补齐缺失字段；
// 解码失败时进入阻断状态。
func (c *Comp[T]) Load(raw bson.Raw) error {
	if len(raw) == 0 {
		return nil
	}
	var data T
	if err := bson.Unmarshal(raw, &data); err != nil {
		c.blocked = true
		c.dirty = false
		return err
	}
	initData(&data)
	c.Data = data
	return nil
}

// Save 生成组件数据的存盘快照。
func (c *Comp[T]) Save() (bson.Raw, error) {
	raw, err := bson.Marshal(&c.Data)
	return bson.Raw(raw), err
}

// PlayerComp 是玩家列表组件：每个玩家一份条目，随玩家上线加载、释放时卸载。
// 能否为玩家新建条目由框架注入的判断函数决定（唯一数据来源为活动服务的已加载玩家集合），
// 组件自身不记录玩家加载状态。
type PlayerComp[T any] struct {
	id        int32
	players   map[uint64]*T
	dirty     map[uint64]bool
	revision  map[uint64]uint64
	blocked   map[uint64]bool       // 存盘数据解码失败的玩家，阻断新建和存盘。
	canCreate func(uid uint64) bool // 是否允许为玩家新建条目，由框架绑定。
}

// NewPlayerComp 创建指定编号的玩家列表组件。
func NewPlayerComp[T any](id int32) *PlayerComp[T] {
	return &PlayerComp[T]{
		id:       id,
		players:  make(map[uint64]*T),
		dirty:    make(map[uint64]bool),
		revision: make(map[uint64]uint64),
		blocked:  make(map[uint64]bool),
	}
}

// CompID 返回组件编号。
func (c *PlayerComp[T]) CompID() int32 { return c.id }

// BindCreateChecker 由框架绑定“能否为玩家新建条目”的判断函数。
func (c *PlayerComp[T]) BindCreateChecker(fn func(uid uint64) bool) { c.canCreate = fn }

// Get 返回玩家条目；条目不存在且框架允许时新建，否则返回 nil（玩家未加载、活动已结束或数据被阻断），
// 避免用默认值覆盖库中数据。
func (c *PlayerComp[T]) Get(uid uint64) *T {
	if c.blocked[uid] {
		return nil
	}
	if data := c.players[uid]; data != nil {
		return data
	}
	if c.canCreate == nil || !c.canCreate(uid) {
		return nil
	}
	data := new(T)
	initData(data)
	c.players[uid] = data
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

// Load 从存盘数据恢复指定玩家的条目，解码成功后调用 Init 补齐缺失字段；
// 解码失败时阻断该玩家条目，Get 返回 nil 且不存盘。
func (c *PlayerComp[T]) Load(uid uint64, raw bson.Raw) error {
	data := new(T)
	if len(raw) > 0 {
		if err := bson.Unmarshal(raw, data); err != nil {
			c.blocked[uid] = true
			delete(c.players, uid)
			delete(c.dirty, uid)
			return err
		}
	}
	initData(data)
	c.players[uid] = data
	return nil
}

// Save 生成指定玩家条目的存盘快照。
func (c *PlayerComp[T]) Save(uid uint64) (bson.Raw, error) {
	raw, err := bson.Marshal(c.players[uid])
	return bson.Raw(raw), err
}

// Unload 卸载指定玩家的条目，并解除阻断。
func (c *PlayerComp[T]) Unload(uid uint64) {
	delete(c.players, uid)
	delete(c.dirty, uid)
	delete(c.revision, uid)
	delete(c.blocked, uid)
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

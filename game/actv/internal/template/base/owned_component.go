package base

// OwnedComp 是个人活动的单条组件，只接受所属玩家访问。
type OwnedComp[T any] struct {
	*Comp[T] // 沿用单条组件加载、标脏和存盘。
	owner    uint64
}

// NewOwnedComp 创建个人活动组件，不建立共享玩家列表。
func NewOwnedComp[T any](id int32, owner uint64) *OwnedComp[T] {
	return &OwnedComp[T]{Comp: NewComp[T](id), owner: owner}
}

// Get 返回所属玩家数据，拒绝跨玩家访问。
func (c *OwnedComp[T]) Get(uid uint64) *T {
	if uid == 0 || uid != c.owner || c.Blocked() {
		return nil
	}
	return c.GetData()
}

// Dirty 标记所属玩家组件变化。
func (c *OwnedComp[T]) Dirty(uid uint64) {
	if uid == c.owner {
		c.MarkDirty()
	}
}

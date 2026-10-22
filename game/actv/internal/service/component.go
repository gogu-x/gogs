package service

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Component 是活动实例的单条数据组件，一个活动实例一份。
// 服务器/联盟活动的单条数据随活动启动加载；玩家独有活动的单条数据随玩家上线加载。
type Component interface {
	CompID() int32              // 组件编号，模板内唯一且不可变更。
	Load(raw bson.Raw) error    // 从存盘数据恢复组件。
	Save() (bson.Raw, error)    // 生成组件存盘快照。
	IsDirty() bool              // 是否需要存盘。
	Revision() uint64           // 当前内存版本号。
	ClearDirty(revision uint64) // 存盘快照版本与当前版本一致时清除脏标记。
}

// PlayerComponent 是活动实例的玩家列表组件，每个玩家一份，随玩家上线加载、释放时卸载。
type PlayerComponent interface {
	CompID() int32                          // 组件编号，模板内唯一且不可变更。
	Load(uid uint64, raw bson.Raw) error    // 从存盘数据恢复指定玩家的条目。
	Save(uid uint64) (bson.Raw, error)      // 生成指定玩家条目的存盘快照。
	SetLoaded(uid uint64)                   // 标记玩家数据已加载，库中无数据时允许新建条目。
	Unload(uid uint64)                      // 卸载指定玩家的条目。
	DirtyPlayers() []uint64                 // 返回有待存盘条目的玩家。
	IsDirty(uid uint64) bool                // 指定玩家条目是否需要存盘。
	Revision(uid uint64) uint64             // 指定玩家条目的内存版本号。
	ClearDirty(uid uint64, revision uint64) // 存盘快照版本与当前版本一致时清除脏标记。
}

// ComponentHolder 由持有数据组件的活动行为实现。
type ComponentHolder interface {
	Components() []Component
	PlayerComponents() []PlayerComponent
}

// ActvDataDoc 是 activity_data 集合中的一条组件数据，与活动元数据 ActvMeta 分开存放。
// 归属者表示该条数据随谁加载：
//   - 单条数据组件：归属者与活动实例一致（服务器/联盟随启动加载，玩家独有活动随玩家上线加载）；
//   - 玩家列表组件：归属者为条目对应的玩家，随该玩家上线加载。
type ActvDataDoc struct {
	ID         string     `bson:"_id"`         // 文档编号，格式为 "{actv_id}_{comp}_{owner_id}"。
	ActvID     int64      `bson:"actv_id"`     // 所属活动实例编号。
	Comp       int32      `bson:"comp"`        // 组件编号。
	OwnerScope OwnerScope `bson:"owner_scope"` // 数据归属者范围。
	OwnerID    OwnerID    `bson:"owner_id"`    // 数据归属者编号。
	Data       bson.Raw   `bson:"data"`        // 组件数据的 BSON 编码。
	revision   uint64     // 生成快照时组件的版本号，不落库。
	dirty      bool       // 快照是否待存盘，不落库。
}

// DataDocID 返回组件数据文档编号。同一活动内组件编号唯一，单条组件只有一个归属者，
// 玩家列表组件的归属者均为玩家，因此 actv_id + comp + owner_id 不会冲突。
func DataDocID(activityID int64, compID int32, ownerID OwnerID) string {
	return fmt.Sprintf("%d_%d_%d", activityID, compID, ownerID)
}

// IsDirty 返回组件数据快照是否待存盘。
func (doc *ActvDataDoc) IsDirty() bool { return doc != nil && doc.dirty }

// Revision 返回生成快照时组件的版本号。
func (doc *ActvDataDoc) Revision() uint64 {
	if doc == nil {
		return 0
	}
	return doc.revision
}

// GetID 返回组件数据文档编号。
func (doc *ActvDataDoc) GetID() string {
	if doc == nil {
		return ""
	}
	return doc.ID
}

func newDataSnapshot(activityID int64, compID int32, scope OwnerScope, ownerID OwnerID, raw bson.Raw, revision uint64) *ActvDataDoc {
	return &ActvDataDoc{
		ID:         DataDocID(activityID, compID, ownerID),
		ActvID:     activityID,
		Comp:       compID,
		OwnerScope: scope,
		OwnerID:    ownerID,
		Data:       raw,
		revision:   revision,
		dirty:      true,
	}
}

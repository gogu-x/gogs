// Package owner 统一管理单个归属对象的活动数据和存盘生命周期。
package owner

import (
	"errors"
	"time"

	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activitypersistence "github.com/gogu-x/gogs/game/activity/internal/persistence"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Data 管理一个玩家、服务器或联盟的活动状态及其存盘生命周期。
type Data struct {
	scope       activitymodel.OwnerScope
	ownerID     uint64
	record      *activitymodel.Record
	persistence *activitypersistence.Model
}

// LegacyPlayerDocument 仅包含迁移旧玩家内嵌活动数据所需的字段。
type LegacyPlayerDocument struct {
	ID       uint64   `bson:"_id"`      // 玩家文档 ID。
	Activity bson.Raw `bson:"activity"` // 旧版玩家文档中的活动管理器。
}

// legacyPlayerModel 描述迁移旧玩家活动数据时读取的玩家文档模型。
type legacyPlayerModel struct{}

// New 为指定归属对象创建空活动数据。
func New(scope activitymodel.OwnerScope, ownerID uint64) *Data {
	return &Data{
		scope:   scope,
		ownerID: ownerID,
		record:  activitymodel.NewRecord(scope, ownerID, activitymodel.NewManager()),
	}
}

// Register 将该归属对象的活动文档模型注册到 Mongo 存储中。
func (data *Data) Register(store *mongorpc.Store) error {
	if data == nil || store == nil {
		return errors.New("activity owner persistence store is unavailable")
	}
	data.persistence = activitypersistence.NewModel(
		func(callback func(*activitymodel.Record) bool) {
			if data.record != nil && callback != nil {
				callback(data.record)
			}
		},
		func(snapshot *activitymodel.Record) {
			current := data.record
			if current != nil && snapshot != nil && current.GetID() == snapshot.GetID() && current.Revision() == snapshot.Revision() {
				current.SetDirty(false)
			}
		},
		time.Minute,
	)
	if err := RegisterLegacyPlayerModel(store); err != nil {
		return err
	}
	return nil
}

// RegisterLegacyPlayerModel 注册读取旧玩家活动字段所需的 Mongo 模型。
func RegisterLegacyPlayerModel(store *mongorpc.Store) error {
	if store == nil {
		return errors.New("activity owner persistence store is unavailable")
	}
	return store.Register(&legacyPlayerModel{})
}

// PersistenceModel 返回该归属对象的活动集合模型。
func (data *Data) PersistenceModel() mongorpc.Model {
	if data == nil {
		return nil
	}
	return data.persistence
}

// Load 加载该归属对象的活动文档，并在需要时迁移旧版内嵌 BSON 数据。
func (data *Data) Load(
	ctx tree.Context,
	store *mongorpc.Store,
	callback func(tree.Context, error),
) bool {
	if data == nil || store == nil || callback == nil || data.ownerID == 0 {
		return false
	}
	return store.LoadOne(
		ctx,
		(*activitymodel.Record)(nil),
		activitymodel.RecordID(data.scope, data.ownerID),
		func(callbackCtx tree.Context, value interface{}, err error) {
			if errors.Is(err, mongo.ErrNoDocuments) {
				err = nil
				value = nil
			}
			if err != nil {
				callback(callbackCtx, err)
				return
			}
			if value != nil {
				record, valid := data.loadedRecord(value, nil)
				if !valid {
					callback(callbackCtx, errors.New("invalid activity owner document"))
					return
				}
				data.record = record
				callback(callbackCtx, nil)
				return
			}
			if data.scope != activitymodel.ScopePlayer {
				record, valid := data.newRecordFromLegacy(nil)
				if !valid {
					callback(callbackCtx, errors.New("empty activity owner record could not be created"))
					return
				}
				data.record = record
				callback(callbackCtx, nil)
				return
			}
			if !store.LoadOne(
				callbackCtx,
				(*LegacyPlayerDocument)(nil),
				data.ownerID,
				func(legacyCtx tree.Context, legacyValue interface{}, legacyErr error) {
					if errors.Is(legacyErr, mongo.ErrNoDocuments) {
						legacyErr = nil
						legacyValue = nil
					}
					if legacyErr != nil {
						callback(legacyCtx, legacyErr)
						return
					}
					legacyData := bson.Raw(nil)
					if legacyDoc, ok := legacyValue.(*LegacyPlayerDocument); ok && legacyDoc != nil {
						legacyData = legacyDoc.Activity
					}
					record, valid := data.newRecordFromLegacy(legacyData)
					if !valid {
						callback(legacyCtx, errors.New("legacy activity document could not be decoded"))
						return
					}
					data.record = record
					callback(legacyCtx, nil)
				},
			) {
				callback(callbackCtx, errors.New("legacy player activity load request could not be sent"))
			}
		},
	)
}

// loadedRecord 校验加载记录的归属信息并补齐缺失的管理器。
func (data *Data) loadedRecord(value interface{}, _ bson.Raw) (*activitymodel.Record, bool) {
	if value != nil {
		record, ok := value.(*activitymodel.Record)
		if !ok || record == nil || record.GetScope() != data.scope || record.GetOwnerID() != data.ownerID ||
			record.GetID() != activitymodel.RecordID(data.scope, data.ownerID) {
			return nil, false
		}
		if record.GetManager() == nil {
			record.SetManager(activitymodel.NewManager())
			record.MarkDirty()
		}
		return record, true
	}
	return data.newRecordFromLegacy(nil)
}

// newRecordFromLegacy 将旧版内嵌活动数据转换为独立活动记录。
func (data *Data) newRecordFromLegacy(legacy bson.Raw) (*activitymodel.Record, bool) {
	manager := activitymodel.NewManager()
	if len(legacy) > 0 {
		if err := bson.Unmarshal(legacy, manager); err != nil {
			return nil, false
		}
	}
	record := activitymodel.NewRecord(data.scope, data.ownerID, manager)
	if len(legacy) > 0 {
		record.MarkDirty()
	}
	return record, true
}

// GetManager 返回该归属对象的活动实例管理器。
func (data *Data) GetManager() *activitymodel.Manager {
	if data == nil || data.record == nil {
		return nil
	}
	return data.record.GetManager()
}

// GetRecord 返回该归属对象的活动存盘记录。
func (data *Data) GetRecord() *activitymodel.Record {
	if data == nil {
		return nil
	}
	return data.record
}

// SetManager 替换该归属对象内存中的活动实例管理器。
func (data *Data) SetManager(manager *activitymodel.Manager) {
	if data == nil || data.record == nil {
		return
	}
	data.record.SetManager(manager)
}

// MarkDirty 将该归属对象的活动文档标记为需要存盘。
func (data *Data) MarkDirty() {
	if data != nil && data.record != nil {
		data.record.MarkDirty()
	}
}

// IsDirty 返回该归属对象的活动文档是否需要存盘。
func (data *Data) IsDirty() bool {
	return data != nil && data.record != nil && data.record.IsDirty()
}

// Sync 根据配置生成活动实例并推进该归属对象的活动生命周期。
func (data *Data) Sync(now int64) ([]activitymodel.Transition, bool) {
	if data == nil || data.record == nil || data.record.GetManager() == nil {
		return nil, false
	}
	manager := data.record.GetManager()
	changed, transitions := activityruntime.Default().Sync(
		manager,
		data.scope,
		data.ownerID,
		activitymodel.DefaultCatalog().List(data.scope),
		now,
	)
	if changed {
		data.record.MarkDirty()
	}
	return transitions, changed
}

// Save 存盘该归属对象的脏活动文档，并按快照版本清理脏标记。
func (data *Data) Save(
	ctx tree.Context,
	store *mongorpc.Store,
	callback func(tree.Context, error),
) bool {
	if data == nil || data.record == nil || data.persistence == nil || store == nil || callback == nil {
		return false
	}
	record := data.record
	if !record.IsDirty() {
		callback(ctx, nil)
		return true
	}
	snapshotValue := data.persistence.Clone(record)
	snapshot, ok := snapshotValue.(*activitymodel.Record)
	if !ok || snapshot == nil {
		return false
	}
	return store.Save(
		ctx,
		(*activitymodel.Record)(nil),
		snapshot.GetID(),
		snapshot,
		func(callbackCtx tree.Context, err error) {
			if err == nil {
				err = data.persistence.AfterSave(callbackCtx, snapshot)
			}
			callback(callbackCtx, err)
		},
	)
}

// GetCollection 返回旧版活动数据所在的玩家集合名称。
func (legacyPlayerModel) GetCollection() string { return "player" }

// GetPrototype 返回旧版玩家活动文档的结构体原型。
func (legacyPlayerModel) GetPrototype() interface{} { return (*LegacyPlayerDocument)(nil) }

// GetIDField 返回旧版玩家文档的主键字段名称。
func (legacyPlayerModel) GetIDField() string { return "_id" }

// GetKey 返回旧版玩家文档的玩家编号。
func (legacyPlayerModel) GetKey(value interface{}) interface{} {
	document, ok := value.(*LegacyPlayerDocument)
	if !ok || document == nil {
		return nil
	}
	return document.ID
}

// Clone 返回旧版迁移读取文档本身。
func (legacyPlayerModel) Clone(value interface{}) interface{} { return value }

// Range 不遍历旧版文档，因为该模型仅用于迁移读取。
func (legacyPlayerModel) Range(func(interface{}) bool) {}

// GetSaveInterval 返回零，表示旧版玩家文档模型不参与自动存盘。
func (legacyPlayerModel) GetSaveInterval() time.Duration { return 0 }

// AfterSave 不处理旧版玩家文档的存盘回调。
func (legacyPlayerModel) AfterSave(tree.Context, interface{}) error { return nil }

// MackDirty 返回旧版玩家文档不需要存盘。
func (legacyPlayerModel) MackDirty(interface{}) bool { return false }

// GetTickerType 返回旧版玩家文档模型使用的定时器类型。
func (legacyPlayerModel) GetTickerType() timer.TimerType { return timer.TimerType(1000007) }

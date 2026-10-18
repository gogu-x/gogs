// Package owner keeps one aggregate's activity data and persistence lifecycle together.
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

// Data owns activity state and persistence for one player, server, or guild.
type Data struct {
	scope       activitymodel.OwnerScope
	ownerID     uint64
	record      *activitymodel.Record
	persistence *activitypersistence.Model
}

// LegacyPlayerDocument exposes only the old embedded activity field for migration reads.
type LegacyPlayerDocument struct {
	ID       uint64   `bson:"_id"`      // 玩家文档 ID。
	Activity bson.Raw `bson:"activity"` // 旧版玩家文档中的活动管理器。
}

type legacyPlayerModel struct{}

// New creates empty activity data for one owner.
func New(scope activitymodel.OwnerScope, ownerID uint64) *Data {
	return &Data{
		scope:   scope,
		ownerID: ownerID,
		record:  activitymodel.NewRecord(scope, ownerID, activitymodel.NewManager()),
	}
}

// Register adds this owner's activity document model to a Mongo store.
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
	if err := store.Register(&legacyPlayerModel{}); err != nil {
		return err
	}
	return nil
}

// PersistenceModel returns this owner's activity collection model.
func (data *Data) PersistenceModel() mongorpc.Model {
	if data == nil {
		return nil
	}
	return data.persistence
}

// Load loads this owner's activity document and migrates an old embedded BSON value when needed.
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

// GetManager returns this owner's activity instances.
func (data *Data) GetManager() *activitymodel.Manager {
	if data == nil || data.record == nil {
		return nil
	}
	return data.record.GetManager()
}

// SetManager replaces this owner's in-memory activity instances.
func (data *Data) SetManager(manager *activitymodel.Manager) {
	if data == nil || data.record == nil {
		return
	}
	data.record.SetManager(manager)
}

// MarkDirty marks this owner's activity document for persistence.
func (data *Data) MarkDirty() {
	if data != nil && data.record != nil {
		data.record.MarkDirty()
	}
}

// IsDirty reports whether this owner's activity document needs persistence.
func (data *Data) IsDirty() bool {
	return data != nil && data.record != nil && data.record.IsDirty()
}

// Sync materializes configured templates and advances this owner's lifecycle.
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

// Save persists this owner's dirty activity document and clears it by snapshot version.
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

func (legacyPlayerModel) GetCollection() string { return "player" }

func (legacyPlayerModel) GetPrototype() interface{} { return (*LegacyPlayerDocument)(nil) }

func (legacyPlayerModel) GetIDField() string { return "_id" }

func (legacyPlayerModel) GetKey(value interface{}) interface{} {
	document, ok := value.(*LegacyPlayerDocument)
	if !ok || document == nil {
		return nil
	}
	return document.ID
}

func (legacyPlayerModel) Clone(value interface{}) interface{} { return value }

func (legacyPlayerModel) Range(func(interface{}) bool) {}

func (legacyPlayerModel) GetSaveInterval() time.Duration { return 0 }

func (legacyPlayerModel) AfterSave(tree.Context, interface{}) error { return nil }

func (legacyPlayerModel) MackDirty(interface{}) bool { return false }

func (legacyPlayerModel) GetTickerType() timer.TimerType { return timer.TimerType(1000007) }

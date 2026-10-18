// Package persistence adapts activity records to Mongo lifecycle storage.
package persistence

import (
	"time"

	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

const activitySaveTickerType = timer.TimerType(1000006)

// Model adapts an owner-scoped activity record source to Mongo persistence.
type Model struct {
	rangeRecords func(func(*activitymodel.Record) bool)
	afterSave    func(*activitymodel.Record)
	interval     time.Duration
}

// NewModel creates a dedicated activity collection model.
func NewModel(
	rangeRecords func(func(*activitymodel.Record) bool),
	afterSave func(*activitymodel.Record),
	interval time.Duration,
) *Model {
	return &Model{rangeRecords: rangeRecords, afterSave: afterSave, interval: interval}
}

// GetCollection returns the dedicated activity collection name.
func (model *Model) GetCollection() string { return "activity" }

// GetPrototype returns the activity record document type.
func (model *Model) GetPrototype() interface{} { return (*activitymodel.Record)(nil) }

// GetIDField returns the activity record document key field.
func (model *Model) GetIDField() string { return "_id" }

// GetKey returns the activity document ID.
func (model *Model) GetKey(data interface{}) interface{} {
	record, ok := data.(*activitymodel.Record)
	if !ok || record == nil {
		return nil
	}
	return record.GetID()
}

// Clone creates a detached activity record snapshot.
func (model *Model) Clone(data interface{}) interface{} {
	record, ok := data.(*activitymodel.Record)
	if !ok || record == nil {
		return nil
	}
	snapshot := record.DeepCopy()
	snapshot.SetDirty(false)
	return snapshot
}

// Range visits activity records from the owning Actor's in-memory store.
func (model *Model) Range(callback func(interface{}) bool) {
	if model == nil || model.rangeRecords == nil || callback == nil {
		return
	}
	model.rangeRecords(func(record *activitymodel.Record) bool {
		if record == nil || !record.IsDirty() {
			return true
		}
		return callback(record)
	})
}

// GetSaveInterval returns the configured periodic persistence interval.
func (model *Model) GetSaveInterval() time.Duration {
	if model == nil {
		return 0
	}
	return model.interval
}

// AfterSave clears dirty state through the owning Actor's version-aware callback.
func (model *Model) AfterSave(_ tree.Context, data interface{}) error {
	snapshot, ok := data.(*activitymodel.Record)
	if !ok || snapshot == nil || model == nil || model.afterSave == nil {
		return nil
	}
	model.afterSave(snapshot)
	return nil
}

// MackDirty reports whether an activity record still needs persistence.
func (model *Model) MackDirty(data interface{}) bool {
	record, ok := data.(*activitymodel.Record)
	return ok && record != nil && record.IsDirty()
}

// GetTickerType returns the Actor timer event used for activity saves.
func (model *Model) GetTickerType() timer.TimerType { return activitySaveTickerType }

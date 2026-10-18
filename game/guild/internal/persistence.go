package internal

import (
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

const guildSaveTickerType = timer.TimerType(1000003)

// PersistenceModel adapts in-memory guild aggregates to Mongo lifecycle storage.
type PersistenceModel struct {
	store *Store
}

// NewPersistenceModel binds persistence callbacks to a guild store.
func NewPersistenceModel(store *Store) *PersistenceModel {
	return &PersistenceModel{store: store}
}

// GetCollection returns the guild document collection name.
func (model *PersistenceModel) GetCollection() string { return "guild" }

// GetPrototype returns the guild persistence type.
func (model *PersistenceModel) GetPrototype() interface{} { return (*Guild)(nil) }

// GetIDField returns the guild document key field.
func (model *PersistenceModel) GetIDField() string { return "_id" }

// GetKey returns a guild's document ID.
func (model *PersistenceModel) GetKey(data interface{}) interface{} {
	guild, ok := data.(*Guild)
	if !ok || guild == nil {
		return nil
	}
	return guild.GetID()
}

// Clone creates a detached guild persistence snapshot.
func (model *PersistenceModel) Clone(data interface{}) interface{} {
	guild, ok := data.(*Guild)
	if !ok || guild == nil {
		return nil
	}
	snapshot := guild.DeepCopy()
	snapshot.SetDirty(false)
	return snapshot
}

// Range visits dirty in-memory guild documents for lifecycle persistence.
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

// GetSaveInterval returns the periodic guild persistence interval.
func (model *PersistenceModel) GetSaveInterval() time.Duration { return time.Minute }

// AfterSave clears dirty state only when the saved snapshot is still current.
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

// MackDirty reports whether the guild still needs to be saved.
func (model *PersistenceModel) MackDirty(data interface{}) bool {
	guild, ok := data.(*Guild)
	return ok && guild != nil && guild.IsDirty()
}

// GetTickerType returns the actor timer event used for periodic guild saves.
func (model *PersistenceModel) GetTickerType() timer.TimerType { return guildSaveTickerType }

package model

import "fmt"

// Record stores one owner's activity state in the dedicated activity collection.
// +deepcopy-gen=true
type Record struct {
	ID       string     `bson:"_id"`      // 归属范围和归属 ID 组成的文档 ID。
	Scope    OwnerScope `bson:"scope"`    // 活动归属范围。
	OwnerID  uint64     `bson:"owner_id"` // 玩家、服务器或联盟 ID。
	Manager  *Manager   `bson:"manager"`  // 该归属对象的活动实例。
	Dirty    bool       `bson:"-"`        // 该活动文档是否需要存盘。
	revision uint64     `bson:"-"`        // 内存版本号，不写入活动文档。
}

// NewRecord creates an activity record for one owner.
func NewRecord(scope OwnerScope, ownerID uint64, manager *Manager) *Record {
	if manager == nil {
		manager = NewManager()
	}
	return &Record{
		ID:      RecordID(scope, ownerID),
		Scope:   scope,
		OwnerID: ownerID,
		Manager: manager,
	}
}

// RecordID returns the stable Mongo document ID for an owner activity record.
func RecordID(scope OwnerScope, ownerID uint64) string {
	return fmt.Sprintf("%d:%d", scope, ownerID)
}

// GetID returns the stable document ID.
func (record *Record) GetID() string {
	if record == nil {
		return ""
	}
	return record.ID
}

// GetScope returns the record owner scope.
func (record *Record) GetScope() OwnerScope {
	if record == nil {
		return ScopeUnspecified
	}
	return record.Scope
}

// GetOwnerID returns the player, server, or guild ID that owns this record.
func (record *Record) GetOwnerID() uint64 {
	if record == nil {
		return 0
	}
	return record.OwnerID
}

// GetManager returns the activity manager stored in this record.
func (record *Record) GetManager() *Manager {
	if record == nil {
		return nil
	}
	return record.Manager
}

// SetManager replaces the activity manager stored in this record.
func (record *Record) SetManager(manager *Manager) {
	if record == nil {
		return
	}
	if manager == nil {
		manager = NewManager()
	}
	record.Manager = manager
}

// MarkDirty marks the activity record for persistence and increments its version.
func (record *Record) MarkDirty() {
	if record == nil {
		return
	}
	record.SetDirty(true)
}

// IsDirty reports whether the activity record needs persistence.
func (record *Record) IsDirty() bool {
	return record != nil && record.Dirty
}

// Revision returns the in-memory version used to protect concurrent saves.
func (record *Record) Revision() uint64 {
	if record == nil {
		return 0
	}
	return record.revision
}

// SetDirty updates the persistence marker.
func (record *Record) SetDirty(dirty bool) {
	if record == nil {
		return
	}
	if dirty {
		record.revision++
	}
	record.Dirty = dirty
}

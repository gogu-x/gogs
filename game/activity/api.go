// Package activity exposes the activity Actor and the stable API used by game modules.
package activity

import (
	"time"

	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityowner "github.com/gogu-x/gogs/game/activity/internal/owner"
	activitypersistence "github.com/gogu-x/gogs/game/activity/internal/persistence"
	activityprotocol "github.com/gogu-x/gogs/game/activity/internal/protocol"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
)

// OwnerScope identifies the aggregate that owns an activity instance.
type OwnerScope = activitymodel.OwnerScope

// Status describes the lifecycle state of an activity instance.
type Status = activitymodel.Status

// Template defines immutable configuration used to create owner-scoped instances.
type Template = activitymodel.Template

// Instance is the mutable activity state owned by a player, server, or guild.
type Instance = activitymodel.Instance

// Transition records one activity lifecycle state change.
type Transition = activitymodel.Transition

// Manager stores activity instances for one owner aggregate.
type Manager = activitymodel.Manager

// Catalog stores immutable activity templates for the running game process.
type Catalog = activitymodel.Catalog

// Record stores one owner's activity state in the dedicated activity collection.
type Record = activitymodel.Record

// LifecycleDefinition binds one template package to its lifecycle hooks.
type LifecycleDefinition = activitylifecycle.Definition

// LifecycleRegistry stores independently implemented activity template hooks.
type LifecycleRegistry = activitylifecycle.Registry

// RuntimeEngine runs template lifecycle hooks against owner-scoped activity data.
type RuntimeEngine = activityruntime.Engine

// PersistenceModel adapts owner-scoped activity records to Mongo persistence.
type PersistenceModel = activitypersistence.Model

// OwnerData owns activity state and persistence for one player, server, or guild.
type OwnerData = activityowner.Data

const (
	// ScopeUnspecified indicates that no activity owner was selected.
	ScopeUnspecified = activitymodel.ScopeUnspecified
	// ScopePlayer assigns an activity instance to one player.
	ScopePlayer = activitymodel.ScopePlayer
	// ScopeServer assigns an activity instance to one game server.
	ScopeServer = activitymodel.ScopeServer
	// ScopeGuild assigns an activity instance to one guild.
	ScopeGuild = activitymodel.ScopeGuild

	// StatusUnspecified represents an invalid or uninitialized lifecycle state.
	StatusUnspecified = activitymodel.StatusUnspecified
	// StatusScheduled represents an activity waiting for its start time.
	StatusScheduled = activitymodel.StatusScheduled
	// StatusStarting represents an activity entering its running phase.
	StatusStarting = activitymodel.StatusStarting
	// StatusRunning represents an activity accepting progress and participation.
	StatusRunning = activitymodel.StatusRunning
	// StatusClosing represents an activity that has stopped accepting progress.
	StatusClosing = activitymodel.StatusClosing
	// StatusClosed represents an activity whose lifecycle is complete.
	StatusClosed = activitymodel.StatusClosed
)

// NewManager creates an empty owner-scoped activity manager.
func NewManager() *Manager { return activitymodel.NewManager() }

// NewOwnerData creates activity data for one aggregate.
func NewOwnerData(scope OwnerScope, ownerID uint64) *OwnerData {
	return activityowner.New(scope, ownerID)
}

// NewRecord creates an activity record for one owner.
func NewRecord(scope OwnerScope, ownerID uint64, manager *Manager) *Record {
	return activitymodel.NewRecord(scope, ownerID, manager)
}

// RecordID returns the stable document ID for an owner activity record.
func RecordID(scope OwnerScope, ownerID uint64) string {
	return activitymodel.RecordID(scope, ownerID)
}

// NewCatalog creates an empty activity template catalog.
func NewCatalog() *Catalog { return activitymodel.NewCatalog() }

// DefaultCatalog returns the process-wide activity template catalog.
func DefaultCatalog() *Catalog { return activitymodel.DefaultCatalog() }

// NewLifecycleRegistry creates an empty lifecycle hook registry.
func NewLifecycleRegistry() *LifecycleRegistry { return activitylifecycle.NewRegistry() }

// DefaultLifecycleRegistry returns the process-wide activity lifecycle registry.
func DefaultLifecycleRegistry() *LifecycleRegistry { return activitylifecycle.DefaultRegistry() }

// NewRuntimeEngine creates an activity runtime using the supplied registry.
func NewRuntimeEngine(registry *LifecycleRegistry) *RuntimeEngine {
	return activityruntime.NewEngine(registry)
}

// DefaultRuntime returns the process-wide activity runtime engine.
func DefaultRuntime() *RuntimeEngine { return activityruntime.Default() }

// NewPersistenceModel creates a dedicated activity collection model.
func NewPersistenceModel(
	rangeRecords func(func(*Record) bool),
	afterSave func(*Record),
	interval time.Duration,
) *PersistenceModel {
	return activitypersistence.NewModel(rangeRecords, afterSave, interval)
}

// ToInfo converts an activity instance to the existing client activity message.
func ToInfo(instance *Instance) *pb_activity.ActivityInfo {
	return activityprotocol.ToInfo(instance)
}

// ToProgress converts an activity instance to the existing progress message.
func ToProgress(instance *Instance) *pb_activity.ActivityProgress {
	return activityprotocol.ToProgress(instance)
}

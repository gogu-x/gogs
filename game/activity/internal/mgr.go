package internal

import (
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityprotocol "github.com/gogu-x/gogs/game/activity/internal/protocol"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/tree"
)

// Mgr owns server-scoped activity instances for one game server.
type Mgr struct {
	serverActivities *activitymodel.Manager
	activityRecord   *activitymodel.Record
	serverID         uint64
}

// NewMgr creates a server activity manager.
func NewMgr(serverID ...uint64) *Mgr {
	currentServerID := uint64(1)
	if len(serverID) > 0 && serverID[0] > 0 {
		currentServerID = serverID[0]
	}
	manager := activitymodel.NewManager()
	return &Mgr{
		serverActivities: manager,
		activityRecord:   activitymodel.NewRecord(activitymodel.ScopeServer, currentServerID, manager),
		serverID:         currentServerID,
	}
}

// GetActivityRecord returns the separately persisted server activity document.
func (mgr *Mgr) GetActivityRecord() *activitymodel.Record {
	if mgr == nil {
		return nil
	}
	return mgr.activityRecord
}

// ServerID returns the server that owns these activities.
func (mgr *Mgr) ServerID() uint64 {
	if mgr == nil {
		return 0
	}
	return mgr.serverID
}

// SetActivityRecord attaches a loaded server activity document to this manager.
func (mgr *Mgr) SetActivityRecord(record *activitymodel.Record) {
	if mgr == nil || record == nil {
		return
	}
	if record.GetManager() == nil {
		record.SetManager(activitymodel.NewManager())
		record.MarkDirty()
	}
	mgr.activityRecord = record
	mgr.serverActivities = record.GetManager()
}

// SyncServerActivities materializes and advances templates owned by this server.
func (mgr *Mgr) SyncServerActivities(now int64) (bool, []activitymodel.Transition) {
	if mgr == nil || mgr.serverActivities == nil {
		return false, nil
	}
	changed, transitions := activityruntime.Default().Sync(
		mgr.serverActivities,
		activitymodel.ScopeServer,
		mgr.serverID,
		activitymodel.DefaultCatalog().List(activitymodel.ScopeServer),
		now,
	)
	if changed && mgr.activityRecord != nil {
		mgr.activityRecord.MarkDirty()
	}
	return changed, transitions
}

// GetList returns server-scoped activity instances for the legacy public activity list.
func (mgr *Mgr) GetList(ctx tree.Context, _ *pb_activity.GetActivityListReq) {
	ctx.Response(mgr.GetActivityListAck(), nil)
}

// GetActivityListAck returns the current server-scoped activity list.
func (mgr *Mgr) GetActivityListAck() *pb_activity.GetActivityListAck {
	activities := make([]*pb_activity.ActivityInfo, 0)
	if mgr != nil && mgr.serverActivities != nil {
		instances := mgr.serverActivities.List()
		activities = make([]*pb_activity.ActivityInfo, 0, len(instances))
		for _, instance := range instances {
			if instance == nil {
				continue
			}
			activities = append(activities, activityprotocol.ToInfo(instance))
		}
	}
	return &pb_activity.GetActivityListAck{Activities: activities}
}

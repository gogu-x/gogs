// Package online contains lifecycle behavior for online activity templates.
package online

import (
	"encoding/json"

	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/tree/tlog"
)

const (
	// HandlerKey identifies the online activity implementation in template metadata.
	HandlerKey       = "online"
	lifecycleVersion = 1
)

type lifecycleState struct {
	Version   int   `json:"version"`    // 当前生命周期数据结构版本。
	CreatedAt int64 `json:"created_at"` // 活动实例首次创建时间。
	StartedAt int64 `json:"started_at"` // 活动首次进入运行态的时间。
	ClosedAt  int64 `json:"closed_at"`  // 活动首次进入关闭阶段的时间。
}

// Register installs the online template lifecycle hooks into the supplied registry.
func Register(registry *activitylifecycle.Registry) bool {
	if registry == nil {
		return false
	}
	return registry.Register(activitylifecycle.Definition{
		Key:          HandlerKey,
		OnCreated:    onCreated,
		OnTransition: onTransition,
	})
}

// BindTemplate assigns this package's lifecycle implementation to a template definition.
func BindTemplate(template *activitymodel.Template) bool {
	if template == nil {
		return false
	}
	template.SetHandler(HandlerKey)
	return true
}

func onCreated(_ *activitymodel.Template, instance *activitymodel.Instance, now int64) bool {
	if instance == nil || len(instance.GetLifecycleData()) > 0 {
		return false
	}
	state := lifecycleState{Version: lifecycleVersion, CreatedAt: now}
	if !saveState(instance, state) {
		return false
	}
	activityID := instance.GetTemplateID()
	scope := instance.GetScope()
	ownerID := instance.GetOwnerID()
	createdAt := state.CreatedAt
	tlog.Log.Info("[activity/templates/online.onCreated] 初始化活动生命周期数据, activity=%v scope=%v owner=%v created_at=%v pid=%v",
		activityID, scope, ownerID, createdAt, playerID(instance))
	return true
}

func onTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
	now int64,
) bool {
	if instance == nil {
		return false
	}
	state := loadState(instance)
	startedAt := state.StartedAt
	closedAt := state.ClosedAt
	beforeStartedAt := startedAt
	beforeClosedAt := closedAt
	switch to {
	case activitymodel.StatusRunning:
		if startedAt > 0 {
			return false
		}
		startedAt = now
	case activitymodel.StatusClosing:
		if closedAt > 0 {
			return false
		}
		closedAt = now
	default:
		return false
	}
	state.StartedAt = startedAt
	state.ClosedAt = closedAt
	if !saveState(instance, state) {
		return false
	}
	activityID := instance.GetTemplateID()
	scope := instance.GetScope()
	ownerID := instance.GetOwnerID()
	afterStartedAt := startedAt
	afterClosedAt := closedAt
	tlog.Log.Info("[activity/templates/online.onTransition] 更新活动生命周期时间戳, activity=%v scope=%v owner=%v from=%v to=%v started_at=%v->%v closed_at=%v->%v pid=%v",
		activityID, scope, ownerID, from, to, beforeStartedAt, afterStartedAt, beforeClosedAt, afterClosedAt, playerID(instance))
	return true
}

func loadState(instance *activitymodel.Instance) lifecycleState {
	state := lifecycleState{Version: lifecycleVersion}
	data := instance.GetLifecycleData()
	if len(data) == 0 {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		tlog.Log.Error("[activity/templates/online.loadState] 活动生命周期数据解析失败, activity=%v scope=%v owner=%v pid=%v err=%v",
			instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), playerID(instance), err)
		return lifecycleState{Version: lifecycleVersion}
	}
	if state.Version <= 0 {
		state.Version = lifecycleVersion
	}
	return state
}

func saveState(instance *activitymodel.Instance, state lifecycleState) bool {
	if instance == nil {
		return false
	}
	data, err := json.Marshal(state)
	if err != nil {
		tlog.Log.Error("[activity/templates/online.saveState] 活动生命周期数据序列化失败, activity=%v scope=%v owner=%v pid=%v err=%v",
			instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), playerID(instance), err)
		return false
	}
	instance.SetLifecycleData(data)
	return true
}

func playerID(instance *activitymodel.Instance) uint64 {
	if instance == nil || instance.GetScope() != activitymodel.ScopePlayer {
		return 0
	}
	return instance.GetOwnerID()
}

// Package online 实现在线活动模板的生命周期行为。
package online

import (
	"encoding/json"

	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityprotocol "github.com/gogu-x/gogs/game/activity/internal/protocol"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/tree/tlog"
)

const (
	// TypeKey 表示在线活动模板绑定的活动类型。
	TypeKey activitymodel.TemplateType = "online"
	// lifecycleVersion 表示在线活动生命周期数据结构的版本。
	lifecycleVersion = 1
)

// lifecycleState 保存在线活动模板独有的生命周期数据。
type lifecycleState struct {
	Version   int   `json:"version"`    // 当前生命周期数据结构版本。
	CreatedAt int64 `json:"created_at"` // 活动实例首次创建时间。
	StartedAt int64 `json:"started_at"` // 活动首次进入运行态的时间。
	ClosedAt  int64 `json:"closed_at"`  // 活动首次进入关闭阶段的时间。
}

// Register 将在线活动模板生命周期方法注册到指定注册表。
func Register(registry *activitylifecycle.Registry) bool {
	if registry == nil {
		return false
	}
	return registry.Register(activitylifecycle.Definition{
		Type:     TypeKey,
		Behavior: templateBehavior{},
	})
}

// templateBehavior 实现在线活动模板要求的全部标准行为。
type templateBehavior struct{}

// BindTemplate 为活动模板指定当前包提供的生命周期实现。
func BindTemplate(template *activitymodel.Template) bool {
	if template == nil {
		return false
	}
	template.SetType(TypeKey)
	return true
}

// OnCreated 初始化在线活动模板独有的数据结构。
func (templateBehavior) OnCreated(
	template *activitymodel.Template,
	instance *activitymodel.Instance,
	now int64,
) bool {
	return onCreated(template, instance, now)
}

// OnTransition 在在线活动状态变化后更新模板独有的数据。
func (templateBehavior) OnTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
	now int64,
) bool {
	return onTransition(instance, from, to, now)
}

// OnTick 处理在线活动的周期性定时行为。
func (templateBehavior) OnTick(_ *activitymodel.Instance, _, _ int64) bool {
	return false
}

// Join 执行在线活动的加入规则。
func (templateBehavior) Join(instance *activitymodel.Instance, _ int64) bool {
	return instance != nil && instance.GetStatus() == activitymodel.StatusRunning
}

// GetInfo 按在线活动模板规则组装活动信息。
func (templateBehavior) GetInfo(instance *activitymodel.Instance) *pb_activity.ActivityInfo {
	return activityprotocol.ToInfo(instance)
}

// GetProgress 按在线活动模板规则组装活动进度。
func (templateBehavior) GetProgress(instance *activitymodel.Instance) *pb_activity.ActivityProgress {
	return activityprotocol.ToProgress(instance)
}

// AddProgress 按在线活动模板规则增加活动进度。
func (templateBehavior) AddProgress(instance *activitymodel.Instance, amount int32, _ int64) bool {
	return instance != nil && instance.AddProgress(amount)
}

// ClaimReward 按在线活动模板规则完成奖励领取。
func (templateBehavior) ClaimReward(instance *activitymodel.Instance, _ int64) bool {
	return instance != nil && instance.ClaimReward()
}

// onCreated 初始化在线活动模板独有的数据结构。
func onCreated(_ *activitymodel.Template, instance *activitymodel.Instance, now int64) bool {
	if instance == nil || len(instance.GetLifecycleData()) > 0 {
		return false
	}
	state := lifecycleState{Version: lifecycleVersion, CreatedAt: now}
	if !saveState(instance, state) {
		return false
	}
	tlog.Log.Info("[activity/templates/online.onCreated] 初始化活动生命周期数据, instance=%v template=%v scope=%v owner=%v created_at=%v",
		instance.GetID(), instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), state.CreatedAt)
	return true
}

// onTransition 在活动状态变化时更新在线活动模板独有的数据。
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
	tlog.Log.Info("[activity/templates/online.onTransition] 更新活动生命周期时间戳, instance=%v template=%v scope=%v owner=%v from=%v to=%v started_at=%v->%v closed_at=%v->%v",
		instance.GetID(), instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), from, to, beforeStartedAt, startedAt, beforeClosedAt, closedAt)
	return true
}

// loadState 读取在线活动模板独有的生命周期数据。
func loadState(instance *activitymodel.Instance) lifecycleState {
	state := lifecycleState{Version: lifecycleVersion}
	data := instance.GetLifecycleData()
	if len(data) == 0 {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		tlog.Log.Error("[activity/templates/online.loadState] 活动生命周期数据解析失败, instance=%v template=%v scope=%v owner=%v err=%v",
			instance.GetID(), instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), err)
		return lifecycleState{Version: lifecycleVersion}
	}
	if state.Version <= 0 {
		state.Version = lifecycleVersion
	}
	return state
}

// saveState 将在线活动模板独有的生命周期数据写入活动实例。
func saveState(instance *activitymodel.Instance, state lifecycleState) bool {
	if instance == nil {
		return false
	}
	data, err := json.Marshal(state)
	if err != nil {
		tlog.Log.Error("[activity/templates/online.saveState] 活动生命周期数据序列化失败, instance=%v template=%v scope=%v owner=%v err=%v",
			instance.GetID(), instance.GetTemplateID(), instance.GetScope(), instance.GetOwnerID(), err)
		return false
	}
	instance.SetLifecycleData(data)
	return true
}

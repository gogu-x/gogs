// Package runtime 按活动实例保存的类型键执行模板行为。
package runtime

import (
	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
)

// Engine 按实例绑定的模板类型执行生命周期和客户端请求行为。
type Engine struct {
	registry *activitylifecycle.Registry
}

// NewEngine 使用指定的类型注册表创建活动运行引擎。
func NewEngine(registry *activitylifecycle.Registry) *Engine {
	return &Engine{registry: registry}
}

// Register 将完整的模板行为绑定到运行引擎的类型注册表。
func (engine *Engine) Register(definition activitylifecycle.Definition) bool {
	if engine == nil || engine.registry == nil {
		return false
	}
	return engine.registry.Register(definition)
}

// Sync 为归属对象创建缺少的活动实例并推进生命周期。
func (engine *Engine) Sync(
	manager *activitymodel.Manager,
	scope activitymodel.OwnerScope,
	ownerID uint64,
	templates []*activitymodel.Template,
	now int64,
) (bool, []activitymodel.Transition) {
	if engine == nil || engine.registry == nil || manager == nil || ownerID == 0 {
		return false, nil
	}
	known := make(map[uint64]*activitymodel.Instance)
	for _, instance := range manager.List() {
		if instance != nil {
			known[instance.GetTemplateID()] = instance
		}
	}
	boundTemplates := make([]*activitymodel.Template, 0, len(templates))
	for _, template := range templates {
		if template == nil || template.GetScope() != scope {
			continue
		}
		if _, found := engine.registry.Get(template.GetType()); found {
			boundTemplates = append(boundTemplates, template)
		}
	}
	created := manager.SyncTemplates(scope, ownerID, boundTemplates, now)
	changed := created
	transitions := make([]activitymodel.Transition, 0)
	for _, template := range boundTemplates {
		if template == nil || template.GetScope() != scope {
			continue
		}
		if known[template.GetID()] != nil {
			continue
		}
		instance, ok := manager.GetByTemplateID(template.GetID())
		if !ok || instance == nil || instance.GetOwnerID() != ownerID {
			continue
		}
		if definition, found := engine.definition(instance); found && definition.Behavior.OnCreated(template, instance, now) {
			changed = true
		}
		if instance.GetStatus() != activitymodel.StatusScheduled {
			transition := newTransition(instance, activitymodel.StatusScheduled, instance.GetStatus())
			if engine.onTransition(instance, transition.GetFrom(), transition.GetTo(), now) {
				changed = true
			}
			transitions = append(transitions, transition)
		}
	}
	advanced, advanceChanged := engine.Advance(manager, now)
	transitions = append(transitions, advanced...)
	if advanceChanged {
		changed = true
	}
	return changed, transitions
}

// Create 创建活动实例并执行其类型绑定的初始化和生命周期行为。
func (engine *Engine) Create(
	manager *activitymodel.Manager,
	template *activitymodel.Template,
	ownerID uint64,
	now int64,
) (*activitymodel.Instance, []activitymodel.Transition, bool) {
	if engine == nil || manager == nil || template == nil || engine.registry == nil {
		return nil, nil, false
	}
	if _, found := engine.registry.Get(template.GetType()); !found {
		return nil, nil, false
	}
	instance, created := manager.Create(template, ownerID, now)
	if !created || instance == nil {
		return nil, nil, false
	}
	transitions := make([]activitymodel.Transition, 0, 2)
	changed := true
	if definition, found := engine.definition(instance); found && definition.Behavior.OnCreated(template, instance, now) {
		changed = true
	}
	if instance.GetStatus() != activitymodel.StatusScheduled {
		transition := newTransition(instance, activitymodel.StatusScheduled, instance.GetStatus())
		if engine.onTransition(instance, transition.GetFrom(), transition.GetTo(), now) {
			changed = true
		}
		transitions = append(transitions, transition)
	}
	advanced, advanceChanged := engine.Advance(manager, now)
	transitions = append(transitions, advanced...)
	if advanceChanged {
		changed = true
	}
	return instance, transitions, changed
}

// Advance 推进已到达时间边界的活动状态并触发模板定时行为。
func (engine *Engine) Advance(
	manager *activitymodel.Manager,
	now int64,
) ([]activitymodel.Transition, bool) {
	if engine == nil || manager == nil {
		return nil, false
	}
	changed := false
	transitions := manager.AdvanceWith(now, func(instance *activitymodel.Instance, from, to activitymodel.Status) {
		changed = true
		if to == activitymodel.StatusRunning {
			instance.SetLastTickAt(now)
		}
		if engine.onTransition(instance, from, to, now) {
			changed = true
		}
	})
	if engine.Tick(manager, now) {
		changed = true
	}
	return transitions, changed
}

// Tick 对到达模板定时周期的运行中实例执行类型绑定的定时行为。
func (engine *Engine) Tick(manager *activitymodel.Manager, now int64) bool {
	if engine == nil || manager == nil {
		return false
	}
	changed := false
	for _, instance := range manager.List() {
		if instance == nil || instance.GetStatus() != activitymodel.StatusRunning {
			continue
		}
		definition, found := engine.definition(instance)
		if !found || definition.TickIntervalSeconds <= 0 {
			continue
		}
		lastTickAt := instance.GetLastTickAt()
		if lastTickAt <= 0 {
			instance.SetLastTickAt(now)
			changed = true
			continue
		}
		if now <= lastTickAt || now-lastTickAt < definition.TickIntervalSeconds {
			continue
		}
		if definition.Behavior.OnTick(instance, now, now-lastTickAt) {
			changed = true
		}
		instance.SetLastTickAt(now)
		changed = true
	}
	return changed
}

// GetInfo 按实例 ID 查找实例并调用其模板类型的信息组装行为。
func (engine *Engine) GetInfo(
	manager *activitymodel.Manager,
	instanceID uint64,
) (*pb_activity.ActivityInfo, bool) {
	instance, definition, found := engine.lookup(manager, instanceID)
	if !found {
		return nil, false
	}
	info := definition.Behavior.GetInfo(instance)
	return info, info != nil
}

// GetInfoList 按实例顺序调用各自模板类型的信息组装行为。
func (engine *Engine) GetInfoList(manager *activitymodel.Manager) []*pb_activity.ActivityInfo {
	if engine == nil || manager == nil {
		return nil
	}
	infos := make([]*pb_activity.ActivityInfo, 0)
	for _, instance := range manager.List() {
		if instance == nil {
			continue
		}
		definition, found := engine.definition(instance)
		if !found {
			continue
		}
		if info := definition.Behavior.GetInfo(instance); info != nil {
			infos = append(infos, info)
		}
	}
	return infos
}

// GetProgress 按实例 ID 查找实例并调用其模板类型的进度组装行为。
func (engine *Engine) GetProgress(
	manager *activitymodel.Manager,
	instanceID uint64,
) (*pb_activity.ActivityProgress, bool) {
	instance, definition, found := engine.lookup(manager, instanceID)
	if !found {
		return nil, false
	}
	progress := definition.Behavior.GetProgress(instance)
	return progress, progress != nil
}

// Join 按实例 ID 查找实例并执行其模板类型的加入规则。
func (engine *Engine) Join(manager *activitymodel.Manager, instanceID uint64, now int64) bool {
	instance, definition, found := engine.lookup(manager, instanceID)
	return found && definition.Behavior.Join(instance, now)
}

// AddProgress 按实例 ID 查找实例并执行其模板类型的进度变更规则。
func (engine *Engine) AddProgress(
	manager *activitymodel.Manager,
	instanceID uint64,
	amount int32,
	now int64,
) bool {
	instance, definition, found := engine.lookup(manager, instanceID)
	return found && definition.Behavior.AddProgress(instance, amount, now)
}

// ClaimReward 按实例 ID 查找实例并执行其模板类型的领奖规则。
func (engine *Engine) ClaimReward(manager *activitymodel.Manager, instanceID uint64, now int64) bool {
	instance, definition, found := engine.lookup(manager, instanceID)
	return found && definition.Behavior.ClaimReward(instance, now)
}

// onTransition 调用实例绑定类型的生命周期状态变化行为。
func (engine *Engine) onTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
	now int64,
) bool {
	definition, found := engine.definition(instance)
	return found && definition.Behavior.OnTransition(instance, from, to, now)
}

// definition 按活动实例保存的类型键读取完整模板行为。
func (engine *Engine) definition(instance *activitymodel.Instance) (activitylifecycle.Definition, bool) {
	if engine == nil || engine.registry == nil || instance == nil {
		return activitylifecycle.Definition{}, false
	}
	return engine.registry.Get(instance.GetType())
}

// lookup 按实例 ID 读取实例，并解析实例绑定的完整模板行为。
func (engine *Engine) lookup(
	manager *activitymodel.Manager,
	instanceID uint64,
) (*activitymodel.Instance, activitylifecycle.Definition, bool) {
	if engine == nil || manager == nil || instanceID == 0 {
		return nil, activitylifecycle.Definition{}, false
	}
	instance, found := manager.Get(instanceID)
	if !found || instance == nil {
		return nil, activitylifecycle.Definition{}, false
	}
	definition, found := engine.definition(instance)
	if !found {
		return nil, activitylifecycle.Definition{}, false
	}
	return instance, definition, true
}

// newTransition 根据活动实例创建生命周期状态变化记录。
func newTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
) activitymodel.Transition {
	if instance == nil {
		return activitymodel.Transition{}
	}
	return activitymodel.Transition{
		InstanceID: instance.GetID(),
		TemplateID: instance.GetTemplateID(),
		Scope:      instance.GetScope(),
		OwnerID:    instance.GetOwnerID(),
		From:       from,
		To:         to,
	}
}

var defaultEngine = NewEngine(activitylifecycle.DefaultRegistry())

// Default 返回进程级活动运行引擎。
func Default() *Engine {
	return defaultEngine
}

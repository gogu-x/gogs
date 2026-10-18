//go:generate go run ../../../../tools/deepcopy-gen -dir ../../../

// Package model defines shared, owner-scoped activity data and lifecycle rules.
package model

import "sort"

// OwnerScope identifies the aggregate that owns an activity instance.
type OwnerScope int32

const (
	// ScopeUnspecified indicates that no activity owner was selected.
	ScopeUnspecified OwnerScope = 0
	// ScopePlayer assigns an activity instance to one player.
	ScopePlayer OwnerScope = 1
	// ScopeServer assigns an activity instance to one game server.
	ScopeServer OwnerScope = 2
	// ScopeGuild assigns an activity instance to one guild.
	ScopeGuild OwnerScope = 3
)

// Status describes the lifecycle state of an activity instance.
type Status int32

const (
	// StatusUnspecified represents an invalid or uninitialized lifecycle state.
	StatusUnspecified Status = 0
	// StatusScheduled represents an activity waiting for its start time.
	StatusScheduled Status = 1
	// StatusStarting represents an activity entering its running phase.
	StatusStarting Status = 2
	// StatusRunning represents an activity that accepts progress and participation.
	StatusRunning Status = 3
	// StatusClosing represents an activity that has stopped accepting progress.
	StatusClosing Status = 4
	// StatusClosed represents an activity whose lifecycle is complete.
	StatusClosed Status = 5
)

// Template defines immutable configuration used to create owner-scoped instances.
type Template struct {
	ID        uint64     // 模板唯一 ID。
	Name      string     // 活动名称。
	Handler   string     // 活动模板实现包注册的生命周期处理器键。
	Scope     OwnerScope // 活动实例归属范围。
	StartTime int64      // Unix 秒级开始时间；0 表示创建后立即开始。
	EndTime   int64      // Unix 秒级结束时间；0 表示不自动结束。
	Target    int32      // 活动完成目标。
}

// GetID returns the template identifier.
func (template *Template) GetID() uint64 {
	if template == nil {
		return 0
	}
	return template.ID
}

// GetName returns the template name.
func (template *Template) GetName() string {
	if template == nil {
		return ""
	}
	return template.Name
}

// GetHandler returns the activity lifecycle handler key.
func (template *Template) GetHandler() string {
	if template == nil {
		return ""
	}
	return template.Handler
}

// SetHandler updates the lifecycle handler key implemented by a template package.
func (template *Template) SetHandler(handler string) {
	if template == nil {
		return
	}
	template.Handler = handler
}

// GetScope returns the template owner scope.
func (template *Template) GetScope() OwnerScope {
	if template == nil {
		return ScopeUnspecified
	}
	return template.Scope
}

// GetStartTime returns the configured Unix start time.
func (template *Template) GetStartTime() int64 {
	if template == nil {
		return 0
	}
	return template.StartTime
}

// GetEndTime returns the configured Unix end time.
func (template *Template) GetEndTime() int64 {
	if template == nil {
		return 0
	}
	return template.EndTime
}

// GetTarget returns the configured activity target.
func (template *Template) GetTarget() int32 {
	if template == nil {
		return 0
	}
	return template.Target
}

// Instance is the mutable activity state owned by a player, server, or guild.
// +deepcopy-gen=true
type Instance struct {
	TemplateID    uint64     `bson:"template_id"`              // 创建该实例的模板 ID。
	Name          string     `bson:"name"`                     // 活动名称快照。
	Handler       string     `bson:"handler,omitempty"`        // 活动模板实现包注册的生命周期处理器键。
	Scope         OwnerScope `bson:"scope"`                    // 活动归属范围。
	OwnerID       uint64     `bson:"owner_id"`                 // 玩家、服务器或联盟 ID。
	Status        Status     `bson:"status"`                   // 当前生命周期状态。
	StartTime     int64      `bson:"start_time"`               // Unix 秒级开始时间。
	EndTime       int64      `bson:"end_time"`                 // Unix 秒级结束时间；0 表示不自动结束。
	LastTickAt    int64      `bson:"last_tick_at,omitempty"`   // 最近一次模板生命周期定时回调的 Unix 时间。
	Progress      int32      `bson:"progress"`                 // 当前活动进度。
	Target        int32      `bson:"target"`                   // 活动完成目标。
	Rewarded      bool       `bson:"rewarded"`                 // 是否已领取活动奖励。
	LifecycleData []byte     `bson:"lifecycle_data,omitempty"` // 模板包独立维护的生命周期状态。
}

// GetTemplateID returns the source template identifier.
func (instance *Instance) GetTemplateID() uint64 {
	if instance == nil {
		return 0
	}
	return instance.TemplateID
}

// GetName returns the instance name snapshot.
func (instance *Instance) GetName() string {
	if instance == nil {
		return ""
	}
	return instance.Name
}

// GetHandler returns the lifecycle handler key copied from the source template.
func (instance *Instance) GetHandler() string {
	if instance == nil {
		return ""
	}
	return instance.Handler
}

// SetHandler updates the lifecycle handler key copied from the source template.
func (instance *Instance) SetHandler(handler string) {
	if instance == nil {
		return
	}
	instance.Handler = handler
}

// GetScope returns the instance owner scope.
func (instance *Instance) GetScope() OwnerScope {
	if instance == nil {
		return ScopeUnspecified
	}
	return instance.Scope
}

// GetOwnerID returns the owning player, server, or guild ID.
func (instance *Instance) GetOwnerID() uint64 {
	if instance == nil {
		return 0
	}
	return instance.OwnerID
}

// GetStatus returns the current lifecycle state.
func (instance *Instance) GetStatus() Status {
	if instance == nil {
		return StatusUnspecified
	}
	return instance.Status
}

// GetStartTime returns the Unix start time.
func (instance *Instance) GetStartTime() int64 {
	if instance == nil {
		return 0
	}
	return instance.StartTime
}

// GetEndTime returns the Unix end time.
func (instance *Instance) GetEndTime() int64 {
	if instance == nil {
		return 0
	}
	return instance.EndTime
}

// GetLastTickAt returns the Unix time of the most recent template tick callback.
func (instance *Instance) GetLastTickAt() int64 {
	if instance == nil {
		return 0
	}
	return instance.LastTickAt
}

// SetLastTickAt updates the Unix time of the most recent template tick callback.
func (instance *Instance) SetLastTickAt(timestamp int64) {
	if instance == nil || timestamp < 0 {
		return
	}
	instance.LastTickAt = timestamp
}

// GetLifecycleData returns a copy of the template-specific persisted state.
func (instance *Instance) GetLifecycleData() []byte {
	if instance == nil {
		return nil
	}
	lifecycleData := instance.LifecycleData
	if len(lifecycleData) == 0 {
		return nil
	}
	data := make([]byte, len(lifecycleData))
	copy(data, lifecycleData)
	return data
}

// SetLifecycleData replaces the template-specific persisted state with a copy.
func (instance *Instance) SetLifecycleData(data []byte) {
	if instance == nil {
		return
	}
	instance.LifecycleData = append([]byte(nil), data...)
}

// GetProgress returns the current progress value.
func (instance *Instance) GetProgress() int32 {
	if instance == nil {
		return 0
	}
	return instance.Progress
}

// GetTarget returns the completion target.
func (instance *Instance) GetTarget() int32 {
	if instance == nil {
		return 0
	}
	return instance.Target
}

// IsRewarded reports whether the reward was already claimed.
func (instance *Instance) IsRewarded() bool {
	return instance != nil && instance.Rewarded
}

// Transition records one lifecycle state change.
type Transition struct {
	TemplateID uint64     // 活动模板 ID。
	Scope      OwnerScope // 活动归属范围。
	OwnerID    uint64     // 玩家、服务器或联盟 ID。
	From       Status     // 变更前状态。
	To         Status     // 变更后状态。
}

// GetTemplateID returns the transitioned activity template ID.
func (transition *Transition) GetTemplateID() uint64 {
	if transition == nil {
		return 0
	}
	return transition.TemplateID
}

// GetOwnerID returns the aggregate that owns the transitioned activity.
func (transition *Transition) GetOwnerID() uint64 {
	if transition == nil {
		return 0
	}
	return transition.OwnerID
}

// GetFrom returns the lifecycle state before the transition.
func (transition *Transition) GetFrom() Status {
	if transition == nil {
		return StatusUnspecified
	}
	return transition.From
}

// GetTo returns the lifecycle state after the transition.
func (transition *Transition) GetTo() Status {
	if transition == nil {
		return StatusUnspecified
	}
	return transition.To
}

// Manager stores activity instances for one owner aggregate.
// +deepcopy-gen=true
type Manager struct {
	Activities map[uint64]*Instance `bson:"activities"` // 按模板 ID 索引的活动实例。
}

// NewManager creates an empty owner-scoped activity manager.
func NewManager() *Manager {
	return &Manager{Activities: make(map[uint64]*Instance)}
}

// Get returns an activity instance by template ID.
func (manager *Manager) Get(templateID uint64) (*Instance, bool) {
	if manager == nil {
		return nil, false
	}
	activities := manager.GetActivities()
	instance, ok := activities[templateID]
	return instance, ok
}

// GetActivities returns the activity map owned by this manager.
func (manager *Manager) GetActivities() map[uint64]*Instance {
	if manager == nil {
		return nil
	}
	return manager.Activities
}

// List returns instances in template ID order.
func (manager *Manager) List() []*Instance {
	if manager == nil {
		return nil
	}
	activitiesByID := manager.GetActivities()
	ids := make([]uint64, 0, len(activitiesByID))
	for templateID := range activitiesByID {
		ids = append(ids, templateID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	instances := make([]*Instance, 0, len(ids))
	for _, templateID := range ids {
		instances = append(instances, activitiesByID[templateID])
	}
	return instances
}

// SyncTemplates creates missing instances for templates owned by this aggregate.
func (manager *Manager) SyncTemplates(
	scope OwnerScope,
	ownerID uint64,
	templates []*Template,
	now int64,
) bool {
	if manager == nil || !validScope(scope) || ownerID == 0 {
		return false
	}
	changed := false
	for _, template := range templates {
		if template == nil || template.GetScope() != scope {
			continue
		}
		if _, created := manager.Create(template, ownerID, now); created {
			changed = true
		}
	}
	return changed
}

// Create adds an instance from a template for the given owner.
func (manager *Manager) Create(template *Template, ownerID uint64, now int64) (*Instance, bool) {
	if manager == nil || template == nil || template.GetID() == 0 || ownerID == 0 ||
		!validScope(template.GetScope()) || template.GetName() == "" || template.GetStartTime() < 0 ||
		template.GetEndTime() < 0 || template.GetTarget() < 0 {
		return nil, false
	}
	activities := manager.GetActivities()
	if activities == nil {
		activities = make(map[uint64]*Instance)
		manager.Activities = activities
	}
	if _, exists := activities[template.GetID()]; exists {
		return nil, false
	}
	startTime := template.GetStartTime()
	if startTime == 0 {
		startTime = now
	}
	endTime := template.GetEndTime()
	if endTime > 0 && endTime <= startTime {
		return nil, false
	}
	instance := &Instance{
		TemplateID: template.GetID(),
		Name:       template.GetName(),
		Handler:    template.GetHandler(),
		Scope:      template.GetScope(),
		OwnerID:    ownerID,
		Status:     StatusScheduled,
		StartTime:  startTime,
		EndTime:    endTime,
		Target:     template.GetTarget(),
	}
	activities[template.GetID()] = instance
	advanceInstance(instance, now)
	return instance, true
}

// Advance updates one lifecycle phase for every instance whose time boundary passed.
func (manager *Manager) Advance(now int64) []Transition {
	return manager.AdvanceWith(now, nil)
}

// AdvanceWith updates lifecycle states and invokes callback after each transition.
func (manager *Manager) AdvanceWith(
	now int64,
	callback func(*Instance, Status, Status),
) []Transition {
	if manager == nil {
		return nil
	}
	transitions := make([]Transition, 0)
	for _, instance := range manager.List() {
		for {
			from, to, changed := advanceInstance(instance, now)
			if !changed {
				break
			}
			if callback != nil {
				callback(instance, from, to)
			}
			transitions = append(transitions, Transition{
				TemplateID: instance.GetTemplateID(),
				Scope:      instance.GetScope(),
				OwnerID:    instance.GetOwnerID(),
				From:       from,
				To:         to,
			})
		}
	}
	return transitions
}

// AddProgress increases progress for a running activity and clamps it to the target.
func (manager *Manager) AddProgress(templateID uint64, amount int32) bool {
	instance, ok := manager.Get(templateID)
	if !ok || instance == nil || instance.GetStatus() != StatusRunning || amount <= 0 {
		return false
	}
	progress := int64(instance.GetProgress()) + int64(amount)
	if target := instance.GetTarget(); target > 0 && progress > int64(target) {
		progress = int64(target)
	}
	if progress == int64(instance.GetProgress()) {
		return false
	}
	instance.Progress = int32(progress)
	return true
}

// Claim marks the activity reward as claimed when its target has been reached.
func (manager *Manager) Claim(templateID uint64) bool {
	instance, ok := manager.Get(templateID)
	if !ok || instance == nil || instance.IsRewarded() || instance.GetTarget() <= 0 || instance.GetProgress() < instance.GetTarget() {
		return false
	}
	if instance.GetStatus() != StatusRunning && instance.GetStatus() != StatusClosed {
		return false
	}
	instance.Rewarded = true
	return true
}

func advanceInstance(instance *Instance, now int64) (Status, Status, bool) {
	if instance == nil {
		return StatusUnspecified, StatusUnspecified, false
	}
	from := instance.GetStatus()
	switch from {
	case StatusScheduled:
		if now >= instance.GetStartTime() {
			instance.Status = StatusStarting
		}
	case StatusStarting:
		instance.Status = StatusRunning
	case StatusRunning:
		if endTime := instance.GetEndTime(); endTime > 0 && now >= endTime {
			instance.Status = StatusClosing
		}
	case StatusClosing:
		instance.Status = StatusClosed
	}
	to := instance.GetStatus()
	return from, to, from != to
}

func validScope(scope OwnerScope) bool {
	return scope == ScopePlayer || scope == ScopeServer || scope == ScopeGuild
}

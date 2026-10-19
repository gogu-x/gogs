//go:generate go run ../../../../tools/deepcopy-gen -dir ../../../

// Package model 定义归属对象持有的活动模板、实例数据和生命周期规则。
package model

import "sort"

const (
	// activityInstanceScopeShift 表示活动实例 ID 中归属范围的位移。
	activityInstanceScopeShift = 62
	// activityInstanceSequenceMask 表示实例 ID 中可分配的序号范围。
	activityInstanceSequenceMask = uint64(1<<activityInstanceScopeShift) - 1
)

// OwnerScope 标识活动实例所属的归属对象类型。
type OwnerScope int32

// TemplateType 标识活动模板实现包提供的模板类型。
type TemplateType string

const (
	// ScopeUnspecified 表示未指定活动归属对象。
	ScopeUnspecified OwnerScope = 0
	// ScopePlayer 表示活动实例归属于单个玩家。
	ScopePlayer OwnerScope = 1
	// ScopeServer 表示活动实例归属于当前游戏服务器。
	ScopeServer OwnerScope = 2
	// ScopeGuild 表示活动实例归属于单个联盟。
	ScopeGuild OwnerScope = 3
)

// Status 描述活动实例当前所处的生命周期状态。
type Status int32

const (
	// StatusUnspecified 表示无效或未初始化的生命周期状态。
	StatusUnspecified Status = 0
	// StatusScheduled 表示活动等待开始时间到达。
	StatusScheduled Status = 1
	// StatusStarting 表示活动正在进入运行阶段。
	StatusStarting Status = 2
	// StatusRunning 表示活动接受参与和进度变更。
	StatusRunning Status = 3
	// StatusClosing 表示活动已停止接受进度变更。
	StatusClosing Status = 4
	// StatusClosed 表示活动生命周期已结束。
	StatusClosed Status = 5
)

// Template 定义创建归属对象活动实例时使用的不可变配置。
type Template struct {
	ID        uint64       // 模板唯一 ID。
	Name      string       // 活动名称。
	Type      TemplateType // 绑定的活动模板类型。
	Scope     OwnerScope   // 活动实例归属范围。
	StartTime int64        // Unix 秒级开始时间；0 表示创建后立即开始。
	EndTime   int64        // Unix 秒级结束时间；0 表示不自动结束。
	Target    int32        // 活动完成目标。
}

// GetID 返回模板唯一 ID。
func (template *Template) GetID() uint64 {
	if template == nil {
		return 0
	}
	return template.ID
}

// GetName 返回活动名称。
func (template *Template) GetName() string {
	if template == nil {
		return ""
	}
	return template.Name
}

// GetType 返回绑定的活动模板类型。
func (template *Template) GetType() TemplateType {
	if template == nil {
		return ""
	}
	return template.Type
}

// SetType 设置活动模板类型。
func (template *Template) SetType(templateType TemplateType) {
	if template == nil {
		return
	}
	template.Type = templateType
}

// GetScope 返回活动实例归属范围。
func (template *Template) GetScope() OwnerScope {
	if template == nil {
		return ScopeUnspecified
	}
	return template.Scope
}

// GetStartTime 返回配置的 Unix 秒级开始时间。
func (template *Template) GetStartTime() int64 {
	if template == nil {
		return 0
	}
	return template.StartTime
}

// GetEndTime 返回配置的 Unix 秒级结束时间。
func (template *Template) GetEndTime() int64 {
	if template == nil {
		return 0
	}
	return template.EndTime
}

// GetTarget 返回活动完成目标。
func (template *Template) GetTarget() int32 {
	if template == nil {
		return 0
	}
	return template.Target
}

// Instance 是由玩家、服务器或联盟持有的可变活动实例数据。
// +deepcopy-gen=true
type Instance struct {
	ID            uint64       `bson:"id"`                       // 活动实例 ID。
	TemplateID    uint64       `bson:"template_id"`              // 创建该实例的模板 ID。
	Name          string       `bson:"name"`                     // 活动名称快照。
	Type          TemplateType `bson:"type"`                     // 活动模板类型。
	Scope         OwnerScope   `bson:"scope"`                    // 活动归属范围。
	OwnerID       uint64       `bson:"owner_id"`                 // 玩家、服务器或联盟 ID。
	Status        Status       `bson:"status"`                   // 当前生命周期状态。
	StartTime     int64        `bson:"start_time"`               // Unix 秒级开始时间。
	EndTime       int64        `bson:"end_time"`                 // Unix 秒级结束时间；0 表示不自动结束。
	LastTickAt    int64        `bson:"last_tick_at,omitempty"`   // 最近一次模板生命周期定时回调的 Unix 时间。
	Progress      int32        `bson:"progress"`                 // 当前活动进度。
	Target        int32        `bson:"target"`                   // 活动完成目标。
	Rewarded      bool         `bson:"rewarded"`                 // 是否已领取活动奖励。
	LifecycleData []byte       `bson:"lifecycle_data,omitempty"` // 模板包独立维护的生命周期状态。
}

// GetID 返回活动实例 ID。
func (instance *Instance) GetID() uint64 {
	if instance == nil {
		return 0
	}
	return instance.ID
}

// SetID 设置活动实例 ID。
func (instance *Instance) SetID(id uint64) {
	if instance == nil || id == 0 {
		return
	}
	instance.ID = id
}

// GetTemplateID 返回创建该实例的模板 ID。
func (instance *Instance) GetTemplateID() uint64 {
	if instance == nil {
		return 0
	}
	return instance.TemplateID
}

// GetName 返回实例创建时保存的活动名称。
func (instance *Instance) GetName() string {
	if instance == nil {
		return ""
	}
	return instance.Name
}

// GetType 返回从模板复制到实例的活动模板类型。
func (instance *Instance) GetType() TemplateType {
	if instance == nil {
		return ""
	}
	return instance.Type
}

// SetType 设置实例绑定的活动模板类型。
func (instance *Instance) SetType(templateType TemplateType) {
	if instance == nil {
		return
	}
	instance.Type = templateType
}

// GetScope 返回活动实例归属范围。
func (instance *Instance) GetScope() OwnerScope {
	if instance == nil {
		return ScopeUnspecified
	}
	return instance.Scope
}

// GetOwnerID 返回玩家、服务器或联盟 ID。
func (instance *Instance) GetOwnerID() uint64 {
	if instance == nil {
		return 0
	}
	return instance.OwnerID
}

// GetStatus 返回当前生命周期状态。
func (instance *Instance) GetStatus() Status {
	if instance == nil {
		return StatusUnspecified
	}
	return instance.Status
}

// GetStartTime 返回 Unix 秒级开始时间。
func (instance *Instance) GetStartTime() int64 {
	if instance == nil {
		return 0
	}
	return instance.StartTime
}

// GetEndTime 返回 Unix 秒级结束时间。
func (instance *Instance) GetEndTime() int64 {
	if instance == nil {
		return 0
	}
	return instance.EndTime
}

// GetLastTickAt 返回最近一次模板定时行为执行时间。
func (instance *Instance) GetLastTickAt() int64 {
	if instance == nil {
		return 0
	}
	return instance.LastTickAt
}

// SetLastTickAt 设置最近一次模板定时行为执行时间。
func (instance *Instance) SetLastTickAt(timestamp int64) {
	if instance == nil || timestamp < 0 {
		return
	}
	instance.LastTickAt = timestamp
}

// GetLifecycleData 返回模板专属持久化状态的副本。
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

// SetLifecycleData 替换模板专属持久化状态。
func (instance *Instance) SetLifecycleData(data []byte) {
	if instance == nil {
		return
	}
	instance.LifecycleData = append([]byte(nil), data...)
}

// GetProgress 返回当前活动进度。
func (instance *Instance) GetProgress() int32 {
	if instance == nil {
		return 0
	}
	return instance.Progress
}

// GetTarget 返回活动完成目标。
func (instance *Instance) GetTarget() int32 {
	if instance == nil {
		return 0
	}
	return instance.Target
}

// IsRewarded 返回活动奖励是否已领取。
func (instance *Instance) IsRewarded() bool {
	return instance != nil && instance.Rewarded
}

// AddProgress 按通用目标规则增加运行中活动的进度。
func (instance *Instance) AddProgress(amount int32) bool {
	if instance == nil || instance.GetStatus() != StatusRunning || amount <= 0 {
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

// ClaimReward 按通用目标规则标记活动奖励已领取。
func (instance *Instance) ClaimReward() bool {
	if instance == nil || instance.IsRewarded() || instance.GetTarget() <= 0 || instance.GetProgress() < instance.GetTarget() {
		return false
	}
	if instance.GetStatus() != StatusRunning && instance.GetStatus() != StatusClosed {
		return false
	}
	instance.Rewarded = true
	return true
}

// Transition 记录一次活动生命周期状态变化。
type Transition struct {
	InstanceID uint64     // 活动实例 ID。
	TemplateID uint64     // 活动模板 ID。
	Scope      OwnerScope // 活动归属范围。
	OwnerID    uint64     // 玩家、服务器或联盟 ID。
	From       Status     // 变更前状态。
	To         Status     // 变更后状态。
}

// GetInstanceID 返回发生状态变化的活动实例 ID。
func (transition *Transition) GetInstanceID() uint64 {
	if transition == nil {
		return 0
	}
	return transition.InstanceID
}

// GetTemplateID 返回发生状态变化的活动模板 ID。
func (transition *Transition) GetTemplateID() uint64 {
	if transition == nil {
		return 0
	}
	return transition.TemplateID
}

// GetOwnerID 返回发生状态变化的活动归属对象 ID。
func (transition *Transition) GetOwnerID() uint64 {
	if transition == nil {
		return 0
	}
	return transition.OwnerID
}

// GetFrom 返回状态变化前的生命周期状态。
func (transition *Transition) GetFrom() Status {
	if transition == nil {
		return StatusUnspecified
	}
	return transition.From
}

// GetTo 返回状态变化后的生命周期状态。
func (transition *Transition) GetTo() Status {
	if transition == nil {
		return StatusUnspecified
	}
	return transition.To
}

// Manager 按活动实例 ID 管理单个归属对象持有的活动实例。
// +deepcopy-gen=true
type Manager struct {
	NextID     uint64               `bson:"next_id"`    // 下一个可分配的归属对象内实例序号。
	Activities map[uint64]*Instance `bson:"activities"` // 按实例 ID 索引的活动实例。
}

// NewManager 创建空的归属对象活动实例管理器。
func NewManager() *Manager {
	return &Manager{Activities: make(map[uint64]*Instance)}
}

// Get 按活动实例 ID 查找实例。
func (manager *Manager) Get(instanceID uint64) (*Instance, bool) {
	if manager == nil {
		return nil, false
	}
	activities := manager.GetActivities()
	instance, ok := activities[instanceID]
	return instance, ok
}

// GetByTemplateID 按模板 ID 查找该模板创建的活动实例。
func (manager *Manager) GetByTemplateID(templateID uint64) (*Instance, bool) {
	if manager == nil || templateID == 0 {
		return nil, false
	}
	for _, instance := range manager.List() {
		if instance != nil && instance.GetTemplateID() == templateID {
			return instance, true
		}
	}
	return nil, false
}

// GetActivities 返回该管理器持有的活动实例映射。
func (manager *Manager) GetActivities() map[uint64]*Instance {
	if manager == nil {
		return nil
	}
	return manager.Activities
}

// List 按实例 ID 顺序返回活动实例。
func (manager *Manager) List() []*Instance {
	if manager == nil {
		return nil
	}
	activitiesByID := manager.GetActivities()
	ids := make([]uint64, 0, len(activitiesByID))
	for instanceID := range activitiesByID {
		ids = append(ids, instanceID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	instances := make([]*Instance, 0, len(ids))
	for _, instanceID := range ids {
		instances = append(instances, activitiesByID[instanceID])
	}
	return instances
}

// SyncTemplates 为归属对象创建尚不存在的模板实例。
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

// Create 为指定归属对象创建活动模板实例。
func (manager *Manager) Create(template *Template, ownerID uint64, now int64) (*Instance, bool) {
	if manager == nil || template == nil || template.GetID() == 0 || template.GetType() == "" || ownerID == 0 ||
		!validScope(template.GetScope()) || template.GetName() == "" || template.GetStartTime() < 0 ||
		template.GetEndTime() < 0 || template.GetTarget() < 0 {
		return nil, false
	}
	activities := manager.GetActivities()
	if activities == nil {
		activities = make(map[uint64]*Instance)
		manager.Activities = activities
	}
	if _, exists := manager.GetByTemplateID(template.GetID()); exists {
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
	if manager.NextID >= activityInstanceSequenceMask {
		return nil, false
	}
	manager.NextID++
	instanceID := (uint64(template.GetScope()) << activityInstanceScopeShift) | manager.NextID
	if _, exists := activities[instanceID]; exists {
		return nil, false
	}
	instance := &Instance{
		ID:         instanceID,
		TemplateID: template.GetID(),
		Name:       template.GetName(),
		Type:       template.GetType(),
		Scope:      template.GetScope(),
		OwnerID:    ownerID,
		Status:     StatusScheduled,
		StartTime:  startTime,
		EndTime:    endTime,
		Target:     template.GetTarget(),
	}
	activities[instance.GetID()] = instance
	advanceInstance(instance, now)
	return instance, true
}

// Advance 推进所有已到达时间边界的活动实例生命周期。
func (manager *Manager) Advance(now int64) []Transition {
	return manager.AdvanceWith(now, nil)
}

// AdvanceWith 推进活动状态并在每次状态变化后调用处理方法。
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
				InstanceID: instance.GetID(),
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

// advanceInstance 按当前时间推进单个活动实例的基础生命周期状态。
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

// validScope 判断活动归属范围是否有效。
func validScope(scope OwnerScope) bool {
	return scope == ScopePlayer || scope == ScopeServer || scope == ScopeGuild
}

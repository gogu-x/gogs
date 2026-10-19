// Package activity 向游戏模块公开活动 Actor 和稳定的活动 API。
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

// OwnerScope 标识活动实例所属的归属对象类型。
type OwnerScope = activitymodel.OwnerScope

// Status 描述活动实例当前所处的生命周期状态。
type Status = activitymodel.Status

// TemplateType 标识活动模板实现包提供的模板类型。
type TemplateType = activitymodel.TemplateType

// Template 定义创建归属对象活动实例时使用的不可变配置。
type Template = activitymodel.Template

// Instance 是由玩家、服务器或联盟持有的可变活动实例数据。
type Instance = activitymodel.Instance

// Transition 记录一次活动生命周期状态变化。
type Transition = activitymodel.Transition

// Manager 按活动实例 ID 管理单个归属对象持有的活动实例。
type Manager = activitymodel.Manager

// Catalog 保存当前游戏进程中的不可变活动模板。
type Catalog = activitymodel.Catalog

// Record 在独立活动集合中保存一个归属对象的活动状态。
type Record = activitymodel.Record

// LifecycleDefinition 将一个模板类型绑定到完整行为实现。
type LifecycleDefinition = activitylifecycle.Definition

// TemplateBehavior 定义活动模板实例必须实现的全部标准行为。
type TemplateBehavior = activitylifecycle.Behavior

// LifecycleRegistry 保存由独立活动模板包实现的类型行为。
type LifecycleRegistry = activitylifecycle.Registry

// RuntimeEngine 按实例类型执行归属对象的活动行为。
type RuntimeEngine = activityruntime.Engine

// PersistenceModel 将归属对象活动记录接入 Mongo 存盘。
type PersistenceModel = activitypersistence.Model

// OwnerData 管理一个玩家、服务器或联盟的活动状态和存盘。
type OwnerData = activityowner.Data

const (
	// ScopeUnspecified 表示未指定活动归属对象。
	ScopeUnspecified = activitymodel.ScopeUnspecified
	// ScopePlayer 表示活动实例归属于单个玩家。
	ScopePlayer = activitymodel.ScopePlayer
	// ScopeServer 表示活动实例归属于当前游戏服务器。
	ScopeServer = activitymodel.ScopeServer
	// ScopeGuild 表示活动实例归属于单个联盟。
	ScopeGuild = activitymodel.ScopeGuild

	// StatusUnspecified 表示无效或未初始化的生命周期状态。
	StatusUnspecified = activitymodel.StatusUnspecified
	// StatusScheduled 表示活动等待开始时间到达。
	StatusScheduled = activitymodel.StatusScheduled
	// StatusStarting 表示活动正在进入运行阶段。
	StatusStarting = activitymodel.StatusStarting
	// StatusRunning 表示活动接受参与和进度变更。
	StatusRunning = activitymodel.StatusRunning
	// StatusClosing 表示活动已停止接受进度变更。
	StatusClosing = activitymodel.StatusClosing
	// StatusClosed 表示活动生命周期已结束。
	StatusClosed = activitymodel.StatusClosed
)

// NewManager 创建空的归属对象活动实例管理器。
func NewManager() *Manager { return activitymodel.NewManager() }

// NewOwnerData 为一个归属对象创建活动数据。
func NewOwnerData(scope OwnerScope, ownerID uint64) *OwnerData {
	return activityowner.New(scope, ownerID)
}

// NewRecord 为一个归属对象创建活动存盘记录。
func NewRecord(scope OwnerScope, ownerID uint64, manager *Manager) *Record {
	return activitymodel.NewRecord(scope, ownerID, manager)
}

// RecordID 返回归属对象活动记录的稳定文档 ID。
func RecordID(scope OwnerScope, ownerID uint64) string {
	return activitymodel.RecordID(scope, ownerID)
}

// NewCatalog 创建空的活动模板目录。
func NewCatalog() *Catalog { return activitymodel.NewCatalog() }

// DefaultCatalog 返回进程级活动模板目录。
func DefaultCatalog() *Catalog { return activitymodel.DefaultCatalog() }

// NewLifecycleRegistry 创建空的活动模板类型注册表。
func NewLifecycleRegistry() *LifecycleRegistry { return activitylifecycle.NewRegistry() }

// DefaultLifecycleRegistry 返回进程级活动模板类型注册表。
func DefaultLifecycleRegistry() *LifecycleRegistry { return activitylifecycle.DefaultRegistry() }

// NewRuntimeEngine 使用指定的类型注册表创建活动运行引擎。
func NewRuntimeEngine(registry *LifecycleRegistry) *RuntimeEngine {
	return activityruntime.NewEngine(registry)
}

// DefaultRuntime 返回进程级活动运行引擎。
func DefaultRuntime() *RuntimeEngine {
	RegisterBuiltins()
	return activityruntime.Default()
}

// NewPersistenceModel 创建独立活动集合的存盘模型。
func NewPersistenceModel(
	rangeRecords func(func(*Record) bool),
	afterSave func(*Record),
	interval time.Duration,
) *PersistenceModel {
	return activitypersistence.NewModel(rangeRecords, afterSave, interval)
}

// ToInfo 将活动实例转换为客户端活动信息。
func ToInfo(instance *Instance) *pb_activity.ActivityInfo {
	return activityprotocol.ToInfo(instance)
}

// ToProgress 将活动实例转换为客户端进度信息。
func ToProgress(instance *Instance) *pb_activity.ActivityProgress {
	return activityprotocol.ToProgress(instance)
}

// Package lifecycle 定义活动模板的完整行为契约和类型注册表。
package lifecycle

import (
	"sync"

	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
)

// Behavior 定义一个活动模板实例必须实现的全部行为。
type Behavior interface {
	// OnCreated 初始化新建活动实例的模板专属数据。
	OnCreated(*activitymodel.Template, *activitymodel.Instance, int64) bool
	// OnTransition 处理活动实例进入新生命周期状态后的模板专属逻辑。
	OnTransition(*activitymodel.Instance, activitymodel.Status, activitymodel.Status, int64) bool
	// OnTick 处理运行中的活动实例到期的模板定时行为。
	OnTick(*activitymodel.Instance, int64, int64) bool
	// Join 执行该活动类型的加入规则。
	Join(*activitymodel.Instance, int64) bool
	// GetInfo 按模板类型组装活动实例的客户端信息。
	GetInfo(*activitymodel.Instance) *pb_activity.ActivityInfo
	// GetProgress 按模板类型组装活动实例的客户端进度。
	GetProgress(*activitymodel.Instance) *pb_activity.ActivityProgress
	// AddProgress 执行该模板的进度变更规则。
	AddProgress(*activitymodel.Instance, int32, int64) bool
	// ClaimReward 执行该模板的奖励领取规则。
	ClaimReward(*activitymodel.Instance, int64) bool
}

// Definition 将一个活动模板类型绑定到唯一的行为实现。
type Definition struct {
	Type                activitymodel.TemplateType // 必须与模板和实例中保存的类型一致。
	TickIntervalSeconds int64                      // 生命周期定时行为的最小间隔；0 表示不触发定时行为。
	Behavior            Behavior                   // 该模板类型实现的完整行为集合。
}

// Registry 保存独立活动模板包注册的类型行为。
type Registry struct {
	mu          sync.RWMutex
	definitions map[activitymodel.TemplateType]Definition
}

// NewRegistry 创建空的活动模板类型注册表。
func NewRegistry() *Registry {
	return &Registry{definitions: make(map[activitymodel.TemplateType]Definition)}
}

// Register 将一个完整的活动模板行为绑定到唯一类型键。
func (registry *Registry) Register(definition Definition) bool {
	templateType := definition.Type
	if registry == nil || templateType == "" || definition.TickIntervalSeconds < 0 || definition.Behavior == nil {
		return false
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	definitions := registry.definitions
	if definitions == nil {
		definitions = make(map[activitymodel.TemplateType]Definition)
		registry.definitions = definitions
	}
	if _, exists := definitions[templateType]; exists {
		return false
	}
	definitions[templateType] = definition
	return true
}

// Get 按类型键读取已注册的活动模板行为。
func (registry *Registry) Get(templateType activitymodel.TemplateType) (Definition, bool) {
	if registry == nil || templateType == "" {
		return Definition{}, false
	}
	registry.mu.RLock()
	definition, ok := registry.definitions[templateType]
	registry.mu.RUnlock()
	return definition, ok
}

var defaultRegistry = NewRegistry()

// DefaultRegistry 返回进程级活动模板类型注册表。
func DefaultRegistry() *Registry {
	return defaultRegistry
}

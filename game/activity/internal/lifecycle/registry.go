// Package lifecycle defines reusable lifecycle hooks for activity template packages.
package lifecycle

import (
	"sync"

	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
)

// Definition binds one template package to its owner-scoped activity lifecycle.
type Definition struct {
	Key                 string                                                                                // 唯一的模板处理器键。
	TickIntervalSeconds int64                                                                                 // 生命周期 Tick 的最小间隔；0 表示不运行 Tick。
	OnCreated           func(*activitymodel.Template, *activitymodel.Instance, int64) bool                    // 实例创建后调用。
	OnTransition        func(*activitymodel.Instance, activitymodel.Status, activitymodel.Status, int64) bool // 状态变化后调用。
	OnTick              func(*activitymodel.Instance, int64, int64) bool                                      // 运行态实例到达 Tick 间隔后调用。
	OnProgress          func(*activitymodel.Instance, int32, int32, int64) bool                               // 通用进度变化后调用。
	OnClaimed           func(*activitymodel.Instance, int64) bool                                             // 通用奖励领取成功后调用。
}

// Registry stores lifecycle definitions registered by independent template packages.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

// NewRegistry creates an empty lifecycle definition registry.
func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]Definition)}
}

// Register adds one template package lifecycle definition.
func (registry *Registry) Register(definition Definition) bool {
	key := definition.Key
	if registry == nil || key == "" || definition.TickIntervalSeconds < 0 {
		return false
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	definitions := registry.definitions
	if definitions == nil {
		definitions = make(map[string]Definition)
		registry.definitions = definitions
	}
	if _, exists := definitions[key]; exists {
		return false
	}
	definitions[key] = definition
	return true
}

// Get returns a lifecycle definition by its registered key.
func (registry *Registry) Get(key string) (Definition, bool) {
	if registry == nil || key == "" {
		return Definition{}, false
	}
	registry.mu.RLock()
	definition, ok := registry.definitions[key]
	registry.mu.RUnlock()
	return definition, ok
}

var defaultRegistry = NewRegistry()

// DefaultRegistry returns the process-wide activity lifecycle registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

package battlestatus

import (
	"github.com/gogu-x/gogs/battle/battle/internal/battlestatus/dot"
	"github.com/gogu-x/gogs/battle/battle/internal/battlestatus/hot"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Registry routes each status kind to its own lifecycle policy.
type Registry map[pb.StatusType]engine.StatusPolicy

// Default 构造内置策略的注册表。
func Default() Registry {
	return Registry{
		pb.StatusType_STATUS_TYPE_DOT: dot.Handler{},
		pb.StatusType_STATUS_TYPE_HOT: hot.Handler{},
	}
}

// Apply 根据配置添加或更新目标状态。
func (r Registry) Apply(statuses []engine.StatusInstance, config engine.StatusConfig) ([]engine.StatusInstance, error) {
	if policy := r[config.Kind]; policy != nil {
		return policy.Apply(statuses, config)
	}
	return append(statuses, engine.StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}

// Tick 计算一次周期状态触发的结果。
func (r Registry) Tick(status engine.StatusInstance) engine.StatusTickResult {
	if policy := r[status.Config.Kind]; policy != nil {
		return policy.Tick(status)
	}
	return engine.StatusTickResult{}
}

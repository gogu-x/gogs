package hot

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

type Handler struct{}

// Apply 根据配置添加或更新目标状态。
func (Handler) Apply(statuses []engine.StatusInstance, config engine.StatusConfig) ([]engine.StatusInstance, error) {
	return append(statuses, engine.StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}

// Tick 计算一次周期状态触发的结果。
func (Handler) Tick(status engine.StatusInstance) engine.StatusTickResult {
	return engine.StatusTickResult{Heal: status.Config.Potency}
}

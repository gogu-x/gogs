package statushandler

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/statushandler/dot"
	"github.com/gogu-x/gogs/battle/battle/internal/statushandler/hot"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
)

// Registry routes each status kind to its own lifecycle policy.
type Registry map[statuskind.Kind]engine.StatusPolicy

func Default() Registry {
	return Registry{
		statuskind.DOT: dot.Handler{},
		statuskind.HOT: hot.Handler{},
	}
}

func (r Registry) Apply(statuses []engine.StatusInstance, config engine.StatusConfig) ([]engine.StatusInstance, error) {
	if policy := r[config.Kind]; policy != nil {
		return policy.Apply(statuses, config)
	}
	return append(statuses, engine.StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}

func (r Registry) Tick(status engine.StatusInstance) engine.StatusTickResult {
	if policy := r[status.Config.Kind]; policy != nil {
		return policy.Tick(status)
	}
	return engine.StatusTickResult{}
}

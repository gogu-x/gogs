package dot

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

type Handler struct{}

func (Handler) Apply(statuses []engine.StatusInstance, config engine.StatusConfig) ([]engine.StatusInstance, error) {
	return append(statuses, engine.StatusInstance{Config: config, Remaining: config.DurationTurns}), nil
}

func (Handler) Tick(status engine.StatusInstance) engine.StatusTickResult {
	return engine.StatusTickResult{Damage: status.Config.Potency, Event: "dot"}
}

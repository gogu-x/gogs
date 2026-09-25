package statushandler

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/statuskind"
)

func (Registry) Has(statuses []engine.StatusInstance, kind statuskind.Kind) bool {
	for _, status := range statuses {
		if status.Remaining > 0 && status.Config.Kind == kind {
			return true
		}
	}
	return false
}

func (Registry) Modifier(statuses []engine.StatusInstance) engine.AttributeModifier {
	var modifier engine.AttributeModifier
	for _, status := range statuses {
		if status.Remaining <= 0 {
			continue
		}
		modifier.AttackFlat += status.Config.Modifier.AttackFlat
		modifier.DefenseFlat += status.Config.Modifier.DefenseFlat
		modifier.SpeedFlat += status.Config.Modifier.SpeedFlat
	}
	return modifier
}

func (Registry) Expire(statuses []engine.StatusInstance) []engine.StatusInstance {
	kept := statuses[:0]
	for _, status := range statuses {
		status.Remaining--
		if status.Remaining > 0 {
			kept = append(kept, status)
		}
	}
	return kept
}

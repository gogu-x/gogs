package battlestatus

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func (Registry) Has(statuses []engine.StatusInstance, kind pb.StatusType) bool {
	for _, status := range statuses {
		if status.IsActive() && status.Config.Kind == kind {
			return true
		}
	}
	return false
}

func (Registry) Modifier(statuses []engine.StatusInstance) engine.AttributeModifier {
	var modifier engine.AttributeModifier
	for _, status := range statuses {
		if !status.IsActive() {
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
		if status.Config.DurationMS > 0 {
			kept = append(kept, status)
			continue
		}
		status.Remaining--
		if status.Remaining > 0 {
			kept = append(kept, status)
		}
	}
	return kept
}

package battlestatus

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Has 判断指定类型的状态是否仍然有效。
func (Registry) Has(statuses []engine.StatusInstance, kind pb.StatusType) bool {
	for _, status := range statuses {
		if status.IsActive() && status.Config.Kind == kind {
			return true
		}
	}
	return false
}

// Modifier 汇总有效状态的属性修正。
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

// Expire 更新状态剩余回合并移除已失效的状态。
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

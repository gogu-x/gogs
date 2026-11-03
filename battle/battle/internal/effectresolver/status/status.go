package status

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// Handler 处理技能附加状态效果。
type Handler struct{}

// Resolve 复制已编译的状态配置。
func (Handler) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	if context.Effect.Status == nil {
		return engine.EffectResolution{}, fmt.Errorf("status config is not resolved: status_id=%s", context.Effect.StatusID)
	}
	status := *context.Effect.Status
	return engine.EffectResolution{Kind: pb.EffectType_EFFECT_TYPE_APPLY_STATUS, Status: &status}, nil
}

// Apply 将状态加入目标并写入状态事件。
func (Handler) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	status := resolution.Status
	if status == nil {
		return fmt.Errorf("resolved status is missing")
	}
	if context.Statuses == nil {
		return fmt.Errorf("status handler is not configured")
	}
	statuses, err := context.Statuses.Apply(context.Target.Statuses(), *status)
	if err != nil {
		return err
	}
	context.Target.SetStatuses(statuses)
	context.Emit(
		pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, status.ID, 0,
		context.Target.HP(), context.Target.HP(), resolution,
	)
	return nil
}

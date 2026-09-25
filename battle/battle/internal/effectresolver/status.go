package effectresolver

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/effectkind"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type ApplyStatus struct{}

func (ApplyStatus) Resolve(context engine.EffectContext) (engine.EffectResolution, error) {
	if context.Effect.Status == nil {
		return engine.EffectResolution{}, fmt.Errorf("status config is not resolved: status_id=%s", context.Effect.StatusID)
	}
	status := *context.Effect.Status
	return engine.EffectResolution{Kind: effectkind.ApplyStatus, Status: &status}, nil
}

func (ApplyStatus) Apply(context engine.EffectApplyContext, resolution engine.EffectResolution) error {
	if resolution.Status == nil {
		return fmt.Errorf("resolved status is missing")
	}
	if context.Statuses == nil {
		return fmt.Errorf("status handler is not configured")
	}
	statuses, err := context.Statuses.Apply(context.Target.Statuses(), *resolution.Status)
	if err != nil {
		return err
	}
	context.Target.SetStatuses(statuses)
	context.Emit(pb.BattleEventType_BATTLE_EVENT_TYPE_STATUS_APPLIED, resolution.Status.ID, 0, context.Target.HP(), context.Target.HP(), resolution.Detail)
	return nil
}

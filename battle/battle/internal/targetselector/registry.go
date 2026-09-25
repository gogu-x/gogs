package targetselector

import (
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/battle/battle/internal/targetselector/allylowest"
	"github.com/gogu-x/gogs/battle/battle/internal/targetselector/enemy"
	"github.com/gogu-x/gogs/battle/battle/internal/targetselector/self"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

type Selector interface {
	Select(engine.TargetQuery) []engine.UnitView
}
type SelectorFunc func(engine.TargetQuery) []engine.UnitView

func (f SelectorFunc) Select(q engine.TargetQuery) []engine.UnitView { return f(q) }

// Registry dispatches target rules to independently implemented selector packages.
type Registry map[pb.TargetRule]Selector

func (r Registry) Select(q engine.TargetQuery) []string {
	selector := r[q.Rule]
	if selector == nil {
		return nil
	}
	selected := selector.Select(q)
	ids := make([]string, 0, len(selected))
	for _, unit := range selected {
		ids = append(ids, unit.ID)
	}
	return ids
}

func Default() Registry {
	return Registry{
		pb.TargetRule_TARGET_RULE_SELF:           self.Selector{},
		pb.TargetRule_TARGET_RULE_ENEMY_SINGLE:   enemy.Single{},
		pb.TargetRule_TARGET_RULE_ENEMY_ALL:      enemy.All{},
		pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP: allylowest.Selector{},
	}
}

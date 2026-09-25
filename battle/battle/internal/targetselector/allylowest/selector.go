package allylowest

import (
	"sort"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
)

type Selector struct{}

func (Selector) Select(query engine.TargetQuery) []engine.UnitView {
	selected := make([]engine.UnitView, 0)
	for _, unit := range query.Units {
		if unit.Alive && unit.Team == query.Actor.Team {
			selected = append(selected, unit)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		if left.HP*right.MaxHP != right.HP*left.MaxHP {
			return left.HP*right.MaxHP < right.HP*left.MaxHP
		}
		if left.Position != right.Position {
			return left.Position < right.Position
		}
		return left.ID < right.ID
	})
	if len(selected) > 1 {
		return selected[:1]
	}
	return selected
}

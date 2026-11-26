package enemy

import (
	"sort"

	"github.com/gogu-x/gogs/battle/battle/internal/engine"
)

type All struct{}
type Single struct{}

// Select 按站位顺序选取存活敌方单位。
func (All) Select(query engine.TargetQuery) []engine.UnitView {
	selected := make([]engine.UnitView, 0)
	for _, unit := range query.Units {
		if unit.Alive && unit.Team != query.Actor.Team {
			selected = append(selected, unit)
		}
	}
	order(selected)
	return selected
}

// Select 选取排序最靠前的存活敌方单位。
func (Single) Select(query engine.TargetQuery) []engine.UnitView {
	selected := (All{}).Select(query)
	if len(selected) > 1 {
		return selected[:1]
	}
	return selected
}

// order 按站位和单位编号稳定排列目标。
func order(units []engine.UnitView) {
	sort.Slice(units, func(i, j int) bool {
		if units[i].Position != units[j].Position {
			return units[i].Position < units[j].Position
		}
		return units[i].ID < units[j].ID
	})
}

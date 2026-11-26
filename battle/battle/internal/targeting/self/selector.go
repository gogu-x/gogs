package self

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

type Selector struct{}

// Select 选取施法单位自身。
func (Selector) Select(query engine.TargetQuery) []engine.UnitView {
	if query.Actor.Alive {
		return []engine.UnitView{query.Actor}
	}
	return nil
}

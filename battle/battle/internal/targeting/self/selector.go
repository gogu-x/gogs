package self

import "github.com/gogu-x/gogs/battle/battle/internal/engine"

type Selector struct{}

func (Selector) Select(query engine.TargetQuery) []engine.UnitView {
	if query.Actor.Alive {
		return []engine.UnitView{query.Actor}
	}
	return nil
}

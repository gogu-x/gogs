// Package runtime runs template lifecycle hooks against owner-scoped activity data.
package runtime

import (
	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
)

// Engine runs registered template hooks for player, server, and guild activity managers.
type Engine struct {
	registry *activitylifecycle.Registry
}

// NewEngine creates an activity runtime using the supplied lifecycle registry.
func NewEngine(registry *activitylifecycle.Registry) *Engine {
	return &Engine{registry: registry}
}

// Register adds a template lifecycle definition to the engine's registry.
func (engine *Engine) Register(definition activitylifecycle.Definition) bool {
	if engine == nil || engine.registry == nil {
		return false
	}
	return engine.registry.Register(definition)
}

// Sync creates missing owner-scoped instances, runs creation hooks, and advances lifecycle.
func (engine *Engine) Sync(
	manager *activitymodel.Manager,
	scope activitymodel.OwnerScope,
	ownerID uint64,
	templates []*activitymodel.Template,
	now int64,
) (bool, []activitymodel.Transition) {
	if engine == nil || manager == nil || ownerID == 0 {
		return false, nil
	}
	known := make(map[uint64]*activitymodel.Instance)
	for _, instance := range manager.List() {
		if instance != nil {
			known[instance.GetTemplateID()] = instance
		}
	}
	created := manager.SyncTemplates(scope, ownerID, templates, now)
	changed := created
	transitions := make([]activitymodel.Transition, 0)
	for _, template := range templates {
		if template == nil || template.GetScope() != scope {
			continue
		}
		if existing := known[template.GetID()]; existing != nil {
			if existing.GetHandler() == "" && template.GetHandler() != "" {
				existing.SetHandler(template.GetHandler())
				changed = true
			}
			continue
		}
		instance, ok := manager.Get(template.GetID())
		if !ok || instance == nil || instance.GetOwnerID() != ownerID {
			continue
		}
		if definition, found := engine.definition(instance); found && definition.OnCreated != nil {
			if definition.OnCreated(template, instance, now) {
				changed = true
			}
		}
		if instance.GetStatus() != activitymodel.StatusScheduled {
			transition := newTransition(instance, activitymodel.StatusScheduled, instance.GetStatus())
			if engine.onTransition(instance, transition.GetFrom(), transition.GetTo(), now) {
				changed = true
			}
			transitions = append(transitions, transition)
		}
	}
	advanced, advanceChanged := engine.Advance(manager, now)
	transitions = append(transitions, advanced...)
	if advanceChanged {
		changed = true
	}
	return changed, transitions
}

// Create creates an instance and runs its package-specific creation and state hooks.
func (engine *Engine) Create(
	manager *activitymodel.Manager,
	template *activitymodel.Template,
	ownerID uint64,
	now int64,
) (*activitymodel.Instance, []activitymodel.Transition, bool) {
	if engine == nil || manager == nil {
		return nil, nil, false
	}
	instance, created := manager.Create(template, ownerID, now)
	if !created || instance == nil {
		return nil, nil, false
	}
	transitions := make([]activitymodel.Transition, 0, 2)
	changed := true
	if definition, found := engine.definition(instance); found && definition.OnCreated != nil {
		if definition.OnCreated(template, instance, now) {
			changed = true
		}
	}
	if instance.GetStatus() != activitymodel.StatusScheduled {
		transition := newTransition(instance, activitymodel.StatusScheduled, instance.GetStatus())
		if engine.onTransition(instance, transition.GetFrom(), transition.GetTo(), now) {
			changed = true
		}
		transitions = append(transitions, transition)
	}
	advanced, advanceChanged := engine.Advance(manager, now)
	transitions = append(transitions, advanced...)
	if advanceChanged {
		changed = true
	}
	return instance, transitions, changed
}

// Advance applies elapsed time boundaries and due template lifecycle ticks.
func (engine *Engine) Advance(
	manager *activitymodel.Manager,
	now int64,
) ([]activitymodel.Transition, bool) {
	if engine == nil || manager == nil {
		return nil, false
	}
	changed := false
	transitions := manager.AdvanceWith(now, func(instance *activitymodel.Instance, from, to activitymodel.Status) {
		changed = true
		if to == activitymodel.StatusRunning {
			instance.SetLastTickAt(now)
		}
		if engine.onTransition(instance, from, to, now) {
			changed = true
		}
	})
	if engine.Tick(manager, now) {
		changed = true
	}
	return transitions, changed
}

// Tick runs due hooks for running instances and persists each next-tick timestamp.
func (engine *Engine) Tick(manager *activitymodel.Manager, now int64) bool {
	if engine == nil || manager == nil {
		return false
	}
	changed := false
	for _, instance := range manager.List() {
		if instance == nil || instance.GetStatus() != activitymodel.StatusRunning {
			continue
		}
		definition, found := engine.definition(instance)
		if !found || definition.OnTick == nil || definition.TickIntervalSeconds <= 0 {
			continue
		}
		lastTickAt := instance.GetLastTickAt()
		if lastTickAt <= 0 {
			instance.SetLastTickAt(now)
			changed = true
			continue
		}
		if now <= lastTickAt || now-lastTickAt < definition.TickIntervalSeconds {
			continue
		}
		if definition.OnTick(instance, now, now-lastTickAt) {
			changed = true
		}
		instance.SetLastTickAt(now)
		changed = true
	}
	return changed
}

// AddProgress applies generic progress and then calls the template's progress hook.
func (engine *Engine) AddProgress(
	manager *activitymodel.Manager,
	templateID uint64,
	amount int32,
	now int64,
) bool {
	if engine == nil || manager == nil {
		return false
	}
	instance, ok := manager.Get(templateID)
	if !ok || instance == nil {
		return false
	}
	previous := instance.GetProgress()
	if !manager.AddProgress(templateID, amount) {
		return false
	}
	if definition, found := engine.definition(instance); found && definition.OnProgress != nil {
		definition.OnProgress(instance, previous, instance.GetProgress(), now)
	}
	return true
}

// Claim marks a valid reward and invokes the template's post-claim hook.
func (engine *Engine) Claim(manager *activitymodel.Manager, templateID uint64, now int64) bool {
	if engine == nil || manager == nil {
		return false
	}
	instance, ok := manager.Get(templateID)
	if !ok || instance == nil || !manager.Claim(templateID) {
		return false
	}
	if definition, found := engine.definition(instance); found && definition.OnClaimed != nil {
		definition.OnClaimed(instance, now)
	}
	return true
}

func (engine *Engine) onTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
	now int64,
) bool {
	if definition, found := engine.definition(instance); found && definition.OnTransition != nil {
		return definition.OnTransition(instance, from, to, now)
	}
	return false
}

func (engine *Engine) definition(instance *activitymodel.Instance) (activitylifecycle.Definition, bool) {
	if engine == nil || engine.registry == nil || instance == nil {
		return activitylifecycle.Definition{}, false
	}
	return engine.registry.Get(instance.GetHandler())
}

func newTransition(
	instance *activitymodel.Instance,
	from activitymodel.Status,
	to activitymodel.Status,
) activitymodel.Transition {
	if instance == nil {
		return activitymodel.Transition{}
	}
	return activitymodel.Transition{
		TemplateID: instance.GetTemplateID(),
		Scope:      instance.GetScope(),
		OwnerID:    instance.GetOwnerID(),
		From:       from,
		To:         to,
	}
}

var defaultEngine = NewEngine(activitylifecycle.DefaultRegistry())

// Default returns the process-wide activity runtime engine.
func Default() *Engine {
	return defaultEngine
}

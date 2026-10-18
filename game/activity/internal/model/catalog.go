package model

import (
	"sort"
	"sync"
)

// Catalog stores immutable activity templates for the running game process.
type Catalog struct {
	mu        sync.RWMutex
	templates map[uint64]*Template
}

// NewCatalog creates an empty activity template catalog.
func NewCatalog() *Catalog {
	return &Catalog{templates: make(map[uint64]*Template)}
}

// Register adds a template when its identity, owner scope, and time range are valid.
func (catalog *Catalog) Register(template *Template) bool {
	if catalog == nil || template == nil || template.GetID() == 0 || template.GetName() == "" ||
		!validScope(template.GetScope()) || template.GetStartTime() < 0 ||
		template.GetEndTime() < 0 || template.GetTarget() < 0 {
		return false
	}
	if template.GetEndTime() > 0 && template.GetStartTime() > 0 && template.GetEndTime() <= template.GetStartTime() {
		return false
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	if catalog.templates == nil {
		catalog.templates = make(map[uint64]*Template)
	}
	if _, exists := catalog.templates[template.GetID()]; exists {
		return false
	}
	copyTemplate := *template
	catalog.templates[template.GetID()] = &copyTemplate
	return true
}

// Get returns a copy of a template by ID.
func (catalog *Catalog) Get(templateID uint64) (*Template, bool) {
	if catalog == nil {
		return nil, false
	}
	catalog.mu.RLock()
	template, ok := catalog.templates[templateID]
	if ok && template != nil {
		copyTemplate := *template
		template = &copyTemplate
	}
	catalog.mu.RUnlock()
	return template, ok
}

// List returns copies of templates matching a scope in ID order.
func (catalog *Catalog) List(scope OwnerScope) []*Template {
	if catalog == nil || !validScope(scope) {
		return nil
	}
	catalog.mu.RLock()
	ids := make([]uint64, 0, len(catalog.templates))
	for id, template := range catalog.templates {
		if template != nil && template.GetScope() == scope {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	templates := make([]*Template, 0, len(ids))
	for _, id := range ids {
		copyTemplate := *catalog.templates[id]
		templates = append(templates, &copyTemplate)
	}
	catalog.mu.RUnlock()
	return templates
}

var defaultCatalog = NewCatalog()

// DefaultCatalog returns the process-wide template catalog.
func DefaultCatalog() *Catalog { return defaultCatalog }

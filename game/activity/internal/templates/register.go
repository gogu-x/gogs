// Package templates registers independent activity template implementations.
package templates

import (
	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	"github.com/gogu-x/gogs/game/activity/internal/templates/online"
)

// Register installs every built-in template implementation.
func Register(registry *activitylifecycle.Registry) bool {
	if registry == nil {
		return false
	}
	return online.Register(registry)
}

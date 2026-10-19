// Package templates 注册独立实现的活动模板类型。
package templates

import (
	activitylifecycle "github.com/gogu-x/gogs/game/activity/internal/lifecycle"
	"github.com/gogu-x/gogs/game/activity/internal/templates/online"
)

// Register 注册所有内置活动模板类型行为。
func Register(registry *activitylifecycle.Registry) bool {
	if registry == nil {
		return false
	}
	return online.Register(registry)
}

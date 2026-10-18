package activity

import (
	"sync"

	activitytemplates "github.com/gogu-x/gogs/game/activity/internal/templates"
	"github.com/gogu-x/tree/tlog"
)

var (
	builtinRegistration sync.Once
	builtinRegistered   bool
)

// RegisterBuiltins registers lifecycle definitions implemented in activity template packages.
func RegisterBuiltins() bool {
	builtinRegistration.Do(func() {
		builtinRegistered = activitytemplates.Register(DefaultLifecycleRegistry())
	})
	return builtinRegistered
}

// RegisterTemplate adds an immutable activity template before or during runtime.
func RegisterTemplate(template *Template) bool {
	if !RegisterBuiltins() {
		tlog.Log.Error("[activity/RegisterTemplate] 注册内置活动模板生命周期失败")
		return false
	}
	activityID := uint64(0)
	handler := ""
	if template != nil {
		activityID = template.GetID()
		handler = template.GetHandler()
	}
	if handler != "" {
		if _, registered := DefaultLifecycleRegistry().Get(handler); !registered {
			tlog.Log.Error("[activity/RegisterTemplate] 活动模板处理器未注册, activity=%v handler=%v",
				activityID, handler)
			return false
		}
	}
	if !DefaultCatalog().Register(template) {
		tlog.Log.Error("[activity/RegisterTemplate] 活动模板注册失败, activity=%v", activityID)
		return false
	}
	return true
}

// Templates returns the configured templates for one owner scope.
func Templates(scope OwnerScope) []*Template {
	return DefaultCatalog().List(scope)
}

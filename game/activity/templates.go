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

// RegisterBuiltins 注册各活动模板包实现的完整行为。
func RegisterBuiltins() bool {
	builtinRegistration.Do(func() {
		builtinRegistered = activitytemplates.Register(DefaultLifecycleRegistry())
	})
	return builtinRegistered
}

// RegisterTemplate 注册绑定到已注册模板类型的不可变活动模板。
func RegisterTemplate(template *Template) bool {
	if !RegisterBuiltins() {
		tlog.Log.Error("[activity/RegisterTemplate] 注册内置活动模板生命周期失败")
		return false
	}
	if template == nil {
		tlog.Log.Error("[activity/RegisterTemplate] 活动模板为空")
		return false
	}
	activityID := template.GetID()
	templateType := template.GetType()
	if templateType == "" {
		tlog.Log.Error("[activity/RegisterTemplate] 活动模板未绑定类型, activity=%v", activityID)
		return false
	}
	if _, registered := DefaultLifecycleRegistry().Get(templateType); !registered {
		tlog.Log.Error("[activity/RegisterTemplate] 活动模板类型未注册, activity=%v type=%v",
			activityID, templateType)
		return false
	}
	if !DefaultCatalog().Register(template) {
		tlog.Log.Error("[activity/RegisterTemplate] 活动模板注册失败, activity=%v", activityID)
		return false
	}
	return true
}

// Templates 返回指定归属范围配置的活动模板。
func Templates(scope OwnerScope) []*Template {
	return DefaultCatalog().List(scope)
}

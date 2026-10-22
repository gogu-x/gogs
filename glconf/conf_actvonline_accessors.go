package glconf

// GetID 返回活动配置编号。
func (cfg *ActvOnlineCfg) GetID() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.ID
}

// GetContentID 返回活动内容编号。
func (cfg *ActvOnlineCfg) GetContentID() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.ContentID
}

// GetAvailable 返回活动配置是否启用。
func (cfg *ActvOnlineCfg) GetAvailable() bool {
	return cfg != nil && cfg.Available
}

// GetType 返回活动模板类型编号。
func (cfg *ActvOnlineCfg) GetType() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.Type
}

// GetTriggerType 返回活动触发时间类型。
func (cfg *ActvOnlineCfg) GetTriggerType() int32 {
	if cfg == nil {
		return 0
	}
	return cfg.TriggerType
}

// GetTriggerVal 返回活动触发时间数值。
func (cfg *ActvOnlineCfg) GetTriggerVal() string {
	if cfg == nil {
		return ""
	}
	return cfg.TriggerVal
}

// IsReopen 返回活动是否重复开启。
func (cfg *ActvOnlineCfg) IsReopen() bool {
	return cfg != nil && cfg.Reopen
}

// GetTriggerRept 返回活动重复触发间隔。
func (cfg *ActvOnlineCfg) GetTriggerRept() string {
	if cfg == nil {
		return ""
	}
	return cfg.TriggerRept
}

// GetDuration 返回活动持续时间。
func (cfg *ActvOnlineCfg) GetDuration() string {
	if cfg == nil {
		return ""
	}
	return cfg.Duration
}

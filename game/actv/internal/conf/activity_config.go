package conf

// ActivityConfig 表示一个活动配置行及其运行时触发信息。
// 源配置行只由对应活动实例的运行时模块持有，不参与活动实例存盘。
type ActivityConfig struct {
	ID                    uint64 // 活动配置行 ID。
	Name                  string // 活动名称。
	Type                  int32  // 绑定的活动模块类型。
	ConfigRow             any    // 配置适配器提供的源配置表行。
	Scope                 int32  // 活动实例归属范围。
	StartTime             int64  // Unix 秒级开始时间；0 表示创建后立即开始。
	EndTime               int64  // Unix 秒级结束时间；0 表示不自动结束。
	OnDemand              bool   // 是否仅由外部触发后创建实例。
	Repeating             bool   // 是否按重复间隔再次触发。
	RepeatIntervalSeconds int64  // 再次触发间隔的秒数。
	Target                int32  // 活动完成目标。
}

// GetID 返回活动配置行 ID。
func (config *ActivityConfig) GetID() uint64 {
	if config == nil {
		return 0
	}
	return config.ID
}

// GetName 返回活动名称。
func (config *ActivityConfig) GetName() string {
	if config == nil {
		return ""
	}
	return config.Name
}

// GetType 返回绑定的活动模块类型。
func (config *ActivityConfig) GetType() int32 {
	if config == nil {
		return 0
	}
	return config.Type
}

// GetScope 返回活动实例归属范围。
func (config *ActivityConfig) GetScope() int32 {
	if config == nil {
		return 1
	}
	return config.Scope
}

// GetStartTime 返回配置的 Unix 秒级开始时间。
func (config *ActivityConfig) GetStartTime() int64 {
	if config == nil {
		return 0
	}
	return config.StartTime
}

// GetEndTime 返回配置的 Unix 秒级结束时间。
func (config *ActivityConfig) GetEndTime() int64 {
	if config == nil {
		return 0
	}
	return config.EndTime
}

// IsOnDemand 返回活动实例是否只由外部触发创建。
func (config *ActivityConfig) IsOnDemand() bool {
	return config != nil && config.OnDemand
}

// IsRepeating 返回配置是否按间隔重复触发。
func (config *ActivityConfig) IsRepeating() bool {
	return config != nil && config.Repeating
}

// GetRepeatIntervalSeconds 返回配置的重复触发间隔秒数。
func (config *ActivityConfig) GetRepeatIntervalSeconds() int64 {
	if config == nil {
		return 0
	}
	return config.RepeatIntervalSeconds
}

// GetTarget 返回活动完成目标。
func (config *ActivityConfig) GetTarget() int32 {
	if config == nil {
		return 0
	}
	return config.Target
}

//go:generate conf-gen

package glconf

// IsReopen 返回活动是否重复开启，保留既有调用名称。
func (cfg *ActvOnlineCfg) IsReopen() bool { return cfg != nil && cfg.GetReopen() }

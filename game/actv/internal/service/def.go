package service

// OwnerScope 标识活动实例所属的归属对象类型。
type OwnerScope int32

// OwnerID 标识活动实例归属对象的编号。
type OwnerID int64

// TemplateType 标识活动代码模块对应的数值类型。
type TemplateType int32

const (
	// ScopeUnspecified 表示未指定活动归属对象。
	ScopeUnspecified OwnerScope = 0
	// ScopePlayer 表示活动实例归属于单个玩家。
	ScopePlayer OwnerScope = 1
	// ScopeServer 表示活动实例归属于当前游戏服务器。
	ScopeServer OwnerScope = 2
	// ScopeGuild 表示活动实例归属于单个联盟。
	ScopeGuild OwnerScope = 3
)

// ActvState 标识活动实例的运行状态。
type ActvState int32

const (
	// StateRunning 表示活动进行中（零值，兼容旧数据）。
	StateRunning ActvState = 0
	// StateEnded 表示活动已结束：实例常驻内存，不再接收 tick 和事件，玩家条目按需结算后卸载。
	StateEnded ActvState = 1
)

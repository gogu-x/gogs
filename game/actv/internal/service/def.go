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

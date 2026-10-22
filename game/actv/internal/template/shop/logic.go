// Package shop 实现商店活动行为。
package shop

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
)

// ShopBehavior 实现商店活动行为；当前没有扩展数据，不注册组件。
type ShopBehavior struct {
	base.BaseBehavior // 通用活动元数据、配置和生命周期行为。
}

// Create 为商店活动实例创建独立的运行时行为对象。
func (*ShopBehavior) Create(actv *service.Actv, cfg conf.ActivityConfig) service.ActvBehavior {
	behavior := &ShopBehavior{}
	behavior.Bind(actv, cfg)
	return behavior
}

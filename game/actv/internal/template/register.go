// Package templates 汇总并注册代码实现的活动模块。
package templates

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/online"
	"github.com/gogu-x/gogs/game/actv/internal/template/shop"
)

const (
	// TypeOnline 在线活动
	TypeOnline service.TemplateType = 1

	// TypeShop 商店
	TypeShop service.TemplateType = 2
)

// Register 注册所有内置活动模块，并声明各模块创建实例的归属范围。新增模板类型时在此追加。
func Register(actvService *service.ActvService) {
	actvService.RegTpl(TypeOnline, &online.Behavior{}, service.ScopeServer)
	actvService.RegTpl(TypeShop, &shop.ShopBehavior{}, service.ScopeServer)
}

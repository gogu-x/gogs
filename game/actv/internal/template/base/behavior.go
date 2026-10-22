// Package base 提供可供活动模块组合使用的通用行为实现。
package base

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
)

// BaseBehavior 为绑定单个活动实例的模块提供默认生命周期、组件注册和通用信息行为。
// 活动模块可嵌入该结构，只覆盖自身需要定制的方法。
type BaseBehavior struct {
	Actv        *service.Actv       // 当前活动实例，元数据修改直接作用于存盘对象。
	ActvCfg     conf.ActivityConfig // 当前活动实例配置。
	comps       []service.Component
	playerComps []service.PlayerComponent
}

// Bind 绑定活动实例和配置，供模板 Create 调用。
func (b *BaseBehavior) Bind(actv *service.Actv, cfg conf.ActivityConfig) {
	b.Actv = actv
	b.ActvCfg = cfg
}

// Meta 返回当前活动实例元数据。
func (b *BaseBehavior) Meta() *service.ActvMeta { return b.Actv.GetMeta() }

// AddComp 注册单条数据组件，需在 OnInit 中调用。
func (b *BaseBehavior) AddComp(comp service.Component) {
	b.comps = append(b.comps, comp)
}

// AddPlayerComp 注册玩家列表组件，需在 OnInit 中调用。
func (b *BaseBehavior) AddPlayerComp(comp service.PlayerComponent) {
	b.playerComps = append(b.playerComps, comp)
}

// Components 返回已注册的单条数据组件。
func (b *BaseBehavior) Components() []service.Component { return b.comps }

// PlayerComponents 返回已注册的玩家列表组件。
func (b *BaseBehavior) PlayerComponents() []service.PlayerComponent { return b.playerComps }

// OnInit 默认不注册组件。
func (*BaseBehavior) OnInit() {}

// OnLoaded 默认不处理组件数据恢复完成。
func (*BaseBehavior) OnLoaded() {}

// OnPlayerLoaded 默认不处理玩家数据加载完成。
func (*BaseBehavior) OnPlayerLoaded(uint64) {}

// OnPlayerUnload 默认不处理玩家数据卸载。
func (*BaseBehavior) OnPlayerUnload(uint64) {}

// OnStart 默认不修改模板专属数据。
func (*BaseBehavior) OnStart() {}

// OnTicker 默认不修改模板专属数据。
func (*BaseBehavior) OnTicker() {}

// OnClosed 默认不执行额外逻辑。
func (*BaseBehavior) OnClosed() {}

// HandleEvent 默认不处理模板未注册的事件。
func (*BaseBehavior) HandleEvent(*sspb.Event) {}

// HandleRequest 默认不处理模板未注册的请求。
func (*BaseBehavior) HandleRequest(interface{}) {}

// OnPack 根据活动元数据和配置组装客户端活动信息。
func (*BaseBehavior) OnPack() *pb_activity.ActivityInfo {
	return &pb_activity.ActivityInfo{}
}

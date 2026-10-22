package service

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"google.golang.org/protobuf/proto"
)

// ActvTemplate 创建指定活动实例和配置对应的活动行为对象。
type ActvTemplate interface {
	Create(actv *Actv, conf conf.ActivityConfig) ActvBehavior
}

// ActvBehavior 定义活动实例的生命周期、信息、请求和事件处理行为。
type ActvBehavior interface {
	OnInit()   // 创建后调用，模板在此注册数据组件。
	OnLoaded() // 组件数据恢复完成后调用。
	// OnPlayerLoaded 玩家上线且其组件数据已恢复到本实例后调用，仅服务器/联盟活动收到。
	OnPlayerLoaded(uid uint64)
	// OnPlayerUnload 玩家即将卸载时调用，仅服务器/联盟活动收到；此时修改的数据会随释放一起存盘。
	OnPlayerUnload(uid uint64)
	OnStart()
	OnTicker()
	OnClosed()
	OnPack() *pb_activity.ActivityInfo
	HandleEvent(event *sspb.Event)
	HandleRequest(uid uint64, request proto.Message)
}

// Settler 由需要在活动结束后结算玩家的模块实现（可选）。
// 已结束的服务器/联盟活动在玩家上线、其数据加载进组件后调用：
// NeedSettle 返回 true 时调用 Settle，结算结果存盘后框架卸载该玩家条目；
// 返回 false 或模块未实现该接口时，框架直接卸载该玩家条目。
// 是否已结算由模块在自己的组件数据中维护。
type Settler interface {
	// NeedSettle 返回玩家是否需要结算。
	NeedSettle(uid uint64) bool
	// Settle 结算玩家：读取组件数据、发奖、修改结算标记并 MarkDirty。
	Settle(uid uint64)
}

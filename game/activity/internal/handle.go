package internal

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
)

// LifecycleTick 表示需要推进一次活动生命周期的内部定时消息。
type LifecycleTick struct{}

// InitMessageHandlers 注册全局活动 Actor 的消息处理方法。
func InitMessageHandlers(
	router *tree.Router,
	onServerEvent func(tree.Context, *sspb.Event),
	onSaveAllActivities func(tree.Context, *ipb.SaveAllActivities),
	onPlayerSessionOpened func(tree.Context, *ipb.PlayerSessionOpened),
	onPlayerSessionClosed func(tree.Context, *ipb.SessionClosed),
	onPlayerActivityMessage func(tree.Context, *ipb.PlayerActivityMessage),
	onPlayerActivityOwnerReleased func(tree.Context, *ipb.PlayerActivityOwnerReleased),
	onLifecycleTick func(tree.Context, *LifecycleTick),
	fallback tree.Handler,
) {
	tree.RegisterFunc(router, &sspb.Event{}, onServerEvent)
	tree.RegisterFunc(router, &ipb.SaveAllActivities{}, onSaveAllActivities)
	tree.RegisterFunc(router, &ipb.PlayerSessionOpened{}, onPlayerSessionOpened)
	tree.RegisterFunc(router, &ipb.SessionClosed{}, onPlayerSessionClosed)
	tree.RegisterFunc(router, &ipb.PlayerActivityMessage{}, onPlayerActivityMessage)
	tree.RegisterFunc(router, &ipb.PlayerActivityOwnerReleased{}, onPlayerActivityOwnerReleased)
	tree.RegisterFunc(router, &LifecycleTick{}, onLifecycleTick)
	router.SetFallback(fallback)
}

// InitRoutes 注册全局活动 Actor 的协议请求路由。
func InitRoutes(
	router *tree.Router,
	onGetActivityList func(tree.Context, *pb_activity.GetActivityListReq),
) {
	tree.RegisterFunc(router, &pb_activity.GetActivityListReq{}, onGetActivityList)
}

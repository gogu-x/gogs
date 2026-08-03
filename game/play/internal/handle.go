package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/game/play/internal/ctl_activity"
	"github.com/gogu-x/gogs/game/play/internal/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl_chat"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

// InitRoutes 注册 play 模块所有路由。
//
// 三类注册语义：
//   - RegisterAnon  ：来自 gate、登录前即可处理（PlayContext.Player 可能为 nil）
//   - RegisterPlayer：来自 gate、要求玩家在线（PlayContext.Player 保证非 nil）
//   - RegisterSys   ：其他模块投递 / 内部异步消息（SysContext，不做登录判断）
func InitRoutes(app *base.App) {
	d := app.Dispatcher()

	// 免登录白名单
	base.RegisterAnon(d, &protoGateway.LoginReq{}, ctl_auth.AutoLogin)
	base.RegisterAnon(d, &protoGateway.RegisterReq{}, ctl_auth.AutoRegister)

	// 需要玩家在线
	base.RegisterPlayer(d, &protoChat.ChatReq{}, ctl_chat.ChatService)
	base.RegisterPlayer(d, &protoActivity.GetActivityListReq{}, ctl_activity.GetActivityList)
	base.RegisterPlayer(d, &protoActivity.JoinActivityReq{}, ctl_activity.JoinActivity)
	base.RegisterPlayer(d, &protoActivity.GetProgressReq{}, ctl_activity.GetProgress)
	base.RegisterPlayer(d, &protoActivity.ClaimRewardReq{}, ctl_activity.ClaimReward)
}

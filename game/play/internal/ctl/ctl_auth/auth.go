package ctl_auth

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// AutoLogin 确认玩家角色已完成数据加载，并返回登录响应。
func AutoLogin(ctx *core.Context, req *pb_auth.LoginGameReq) {
	if ctx == nil || req == nil {
		tlog.Log.Error("[ctl_auth/AutoLogin] 登录请求或上下文为空")
		return
	}
	playerID := ctx.PlayerID()
	uid := req.GetUID()
	if uid == 0 || uid != playerID {
		tlog.Log.Error("[ctl_auth/AutoLogin] 登录 UID 校验失败, pid=%v requestUID=%v", playerID, uid)
		ctx.CastPlayerMsg(playerID, &pb_auth.LoginGameAck{
			Code: pb_common.ErrCode_PARAM,
			Uid:  playerID,
		})
		return
	}
	arg := comm.NewArg()
	arg.Set("playerId", uid)
	ctx.Emit(core.PlayerOnLogin, arg)
	if !ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_OK, Uid: uid}) {
		tlog.Log.Error("[ctl_auth/AutoLogin] 登录响应未能投递到网关, pid=%v", uid)
	}
	tlog.Log.Info("[ctl_auth/AutoLogin] 玩家登录完成, pid=%v", uid)
}

// OnLogout 处理当前玩家角色的会话退出事件。
func OnLogout(actor *core.PlayerActor, _ *comm.Arg) {
	if actor == nil {
		return
	}
	currentPlayer := actor.GetPlayer()
	if currentPlayer == nil {
		return
	}
	tlog.Log.Info("[ctl_auth/OnLogout] 玩家退出事件已处理, pid=%v", currentPlayer.GetUID())
}

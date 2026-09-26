package ctl_auth

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/loader"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// AutoLogin 登录时加载一次玩家数据，数据就绪后再注册玩家会话。
func AutoLogin(ctx *core.Context, req *pb_auth.LoginGameReq) {
	uid := req.GetUID()
	if uid == 0 {
		ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_PARAM})
		return
	}
	play := ctx.Play
	if play.GetPlayerMgr() == nil {
		ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_INTERNAL, Uid: uid})
		return
	}
	tlog.Log.Info("[ctl_auth/AutoLogin] 开始加载玩家数据, pid=%v", uid)
	if !loader.OnLoadPlayer(play, uid, func(p *core.Play, loaded *player.Player, err error) {
		if play.IsShuttingDown() {
			ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_INTERNAL, Uid: uid})
			return
		}
		if err != nil {
			tlog.Log.Error("[ctl_auth/AutoLogin] 玩家数据加载失败, pid=%v err=%v", uid, err)
			ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_INTERNAL, Uid: uid})
			return
		}
		if loaded == nil {
			loaded = player.NewPlayerData(uid)
		}

		onLogin(play, uid, loaded)
		ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_OK, Uid: uid})
	}) {
		ctx.CastPlayerMsg(uid, &pb_auth.LoginGameAck{Code: pb_common.ErrCode_INTERNAL, Uid: uid})
	}
}

func onLogin(play *core.Play, uid uint64, p *player.Player) {
	playerMgr := play.GetPlayerMgr()
	playerMgr.Add(p)
	arg := comm.NewArg()
	arg.Set("playerId", uid)
	play.Emit(core.PlayerOnLogin, arg)
	tlog.Log.Info("[ctl_auth/AutoLogin] 玩家登录完成, pid=%v online=%v", uid, playerMgr.Count())
}

func OnLogout(play *core.Play, arg *comm.Arg) {

}

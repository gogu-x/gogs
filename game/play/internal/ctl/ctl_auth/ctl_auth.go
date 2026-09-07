package ctl_auth

import (
	"github.com/gogu-x/gogs/game/play/internal/context"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
func AutoLogin(ctx *context.Context, req *pb_auth.LoginGameReq) {
	playerId := req.UID
	ack := &pb_auth.LoginGameAck{Code: pb_common.ErrCode_PARAM}
	defer ctx.CastPlayerIdMsg(playerId, ack)

	if req.UID == 0 {
		return
	}

	//触发登录事件
	arg := comm.NewArg()
	arg.Set("ctx", *ctx)
	arg.Set("playerId", playerId)
	ctx.Event().Emit(context.PlayerOnLogin, arg)
	ack.Code = pb_common.ErrCode_OK
	ack.Uid = playerId
}

func AutoRegister(ctx *context.Context, msg interface{}, err error) {
	if err != nil {
		tlog.Log.Error("auto register failed: %v", err)
		return
	}
	tlog.Log.Debug("auto register response: %T", msg)
}

package ctl_auth

import (
	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/gogs/pb/pb_auth"
	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree/comm"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
func AutoLogin(ctx *common.Context, req *pb_auth.LoginGameReq) {
	playerId := req.UID
	ack := &pb_auth.LoginGameAck{Code: pb_common.ErrCode_ERR_PARAM}
	defer ctx.CastPlayerIdMsg(playerId, ack)

	if req.UID == 0 {
		return
	}

	//触发登录事件
	arg := comm.NewArg()
	arg.Set("playerId", playerId)
	ctx.Event().Emit(common.PlayerOnLogin, arg)
	ack.Code = pb_common.ErrCode_OK
}

func AutoRegister(s *common.Context, _ *pb_gateway.RegisterReq) {
}

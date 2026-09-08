package ctl_auth

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
func AutoLogin(ctx *core.Context, req *pb_auth.LoginGameReq) {
	playerId := req.UID
	ack := &pb_auth.LoginGameAck{Code: pb_common.ErrCode_PARAM}
	defer ctx.CastPlayerIdMsg(playerId, ack)

	if req.UID == 0 {
		return
	}
	if ctx.GetPlayer(playerId) == nil {
		ctx.Players().Add(player.NewPlayerData(playerId))
	}

	//触发登录事件。事件参数只传值类型：*core.Context 生命周期仅限本条消息，
	//监听者一旦留存就会拿到失效的请求上下文。
	//Context.Emit 会把当前请求上下文透传给监听者，因此监听者内部能力齐全。
	arg := comm.NewArg()
	arg.Set("playerId", playerId)
	ctx.Emit(core.PlayerOnLogin, arg)
	ack.Code = pb_common.ErrCode_OK
	ack.Uid = playerId
}

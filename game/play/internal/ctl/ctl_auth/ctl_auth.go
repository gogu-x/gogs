package ctl_auth

import (
	"log"

	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/pb_auth"
	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/gogs/pb/pb_gateway"
	"github.com/gogu-x/tree/comm"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
func AutoLogin(ctx *common.PlayerContext, req *pb_auth.LoginGameReq) {
	playerId := req.UID
	if req.UID == 0 {
		ctx.Reply(&pb_gateway.LoginAck{Code: pb_common.ErrCode_ERR_PARAM, Msg: "missing uid"})
		return
	}

	p := ctx.Players().Get(playerId)
	if p == nil {
		p = player.NewPlayerData(playerId)
		ctx.Players().Add(p)
		log.Printf("play: uid=%d session created, online=%d", playerId, ctx.Players().Count())
	}
	ctx.Player = p

	//触发登录事件
	arg := comm.NewArg()
	arg.Set("playerId", playerId)
	ctx.Event().Emit(common.PlayerOnLogin, arg)

	ctx.Reply(&pb_gateway.LoginAck{Code: pb_common.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(s *common.PlayerContext, _ *pb_gateway.RegisterReq) {
	s.Reply(&pb_gateway.RegisterAck{Code: pb_common.ErrCode_OK, Msg: "ok"})
}

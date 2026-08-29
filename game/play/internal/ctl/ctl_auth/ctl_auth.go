package ctl_auth

import (
	"log"

	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree/comm"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
// Gate 已完成鉴权（uid 由 Gate 写入 Frame），这里只负责准备内存数据。
func AutoLogin(s *base.PlayContext, _ *protoGateway.LoginGameReq) {
	if s.PlayerId == 0 {
		s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_PARAM, Msg: "missing uid"})
		return
	}

	p := s.Players().Get(s.PlayerId)
	if p == nil {
		p = player.NewPlayerData(s.PlayerId)
		s.Players().Add(p)
		log.Printf("play: uid=%d session created, online=%d", s.PlayerId, s.Players().Count())
	}
	s.Player = p

	//触发登录事件
	arg := comm.NewArg()
	arg.Set("playerId", s.PlayerId)
	s.App.Event().Emit(base.PlayerOnLogin, arg)

	s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(s *base.PlayContext, _ *protoGateway.RegisterReq) {
	s.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

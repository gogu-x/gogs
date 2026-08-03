package internal

import (
	"log"

	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/game/play/module/player"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
// Gate 已完成鉴权（uid 由 Gate 写入 Frame），这里只负责准备内存数据。
func AutoLogin(s *base.PlayContext, _ *protoGateway.LoginReq) {
	if s.UID == 0 {
		s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_ERR_PARAM, Msg: "missing uid"})
		return
	}

	p := s.Players().Get(s.UID)
	if p == nil {
		// TODO: 先从 MongoDB 加载，加载不到再新建
		p = player.NewPlayerData(s.UID)
		s.Players().Add(p)
		log.Printf("play: uid=%d session created, online=%d", s.UID, s.Players().Count())
	}
	s.Player = p

	s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(s *base.PlayContext, _ *protoGateway.RegisterReq) {
	s.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

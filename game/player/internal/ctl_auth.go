package internal

import (
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func AutoLogin(s *Session, msg interface{}) {
	_ = msg.(*protoGateway.LoginReq)
	s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(s *Session, msg interface{}) {
	_ = msg.(*protoGateway.RegisterReq)
	s.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

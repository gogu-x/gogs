package internal

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func AutoLogin(s *Session, _ actor.ActorContext, msg interface{}) {
	_ = msg.(*protoGateway.LoginReq)
	s.Reply(&protoGateway.LoginAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

func AutoRegister(s *Session, _ actor.ActorContext, msg interface{}) {
	_ = msg.(*protoGateway.RegisterReq)
	s.Reply(&protoGateway.RegisterAck{Code: protoCommon.ErrCode_OK, Msg: "ok"})
}

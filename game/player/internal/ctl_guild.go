package internal

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoCommon"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func CreateGuild(s *Session, msg interface{}) {
	req := msg.(*protoGuild.CreateGuildReq)
	req.Uid = s.Data.UID
	req.LeaderName = s.Data.Name
	req.LeaderLevel = s.Data.Level
	requestGuild(s, req, func(ret interface{}, err error) {
		if err != nil {
			s.Reply(&protoGuild.CreateGuildAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
			return
		}
		s.Reply(ret.(*protoGuild.CreateGuildAck))
	})
}

func JoinGuild(s *Session, msg interface{}) {
	req := msg.(*protoGuild.JoinGuildReq)
	req.Uid = s.Data.UID
	req.MemberName = s.Data.Name
	req.MemberLevel = s.Data.Level
	requestGuild(s, req, func(ret interface{}, err error) {
		if err != nil {
			s.Reply(&protoGuild.JoinGuildAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
			return
		}
		s.Reply(ret.(*protoGuild.JoinGuildAck))
	})
}

func LeaveGuild(s *Session, msg interface{}) {
	requestGuild(s, &protoGuild.LeaveGuildReq{Uid: s.Data.UID}, func(ret interface{}, err error) {
		if err != nil {
			s.Reply(&protoGuild.LeaveGuildAck{Code: protoCommon.ErrCode_ERR_UNKNOWN, Msg: err.Error()})
			return
		}
		s.Reply(ret.(*protoGuild.LeaveGuildAck))
	})
}

func GetGuild(s *Session, msg interface{}) {
	requestGuild(s, msg.(*protoGuild.GetGuildReq), func(ret interface{}, err error) {
		if err != nil {
			s.Reply(&protoGuild.GetGuildAck{Code: protoCommon.ErrCode_ERR_UNKNOWN})
			return
		}
		s.Reply(ret.(*protoGuild.GetGuildAck))
	})
}

func requestGuild(s *Session, msg interface{ ProtoMessage() }, cb func(interface{}, error)) {
	s.Request(actor.MustLookup(constant.ActorGuild), msg, func(ret interface{}, err error) {
		if err != nil {
			log.Printf("requestGuild error: %v", err)
		}
		cb(ret, err)
	})
}

package internal

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoActivity"
)

func GetActivityList(s *Session, msg interface{}) {
	requestActivity(s, msg.(*protoActivity.GetActivityListReq), func(ret interface{}, err error) {
		s.Reply(ret.(*protoActivity.GetActivityListAck))
	})
}

func JoinActivity(s *Session, msg interface{}) {
	req := msg.(*protoActivity.JoinActivityReq)
	req.Uid = s.Data.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		s.Reply(ret.(*protoActivity.JoinActivityAck))
	})
}

func GetProgress(s *Session, msg interface{}) {
	req := msg.(*protoActivity.GetProgressReq)
	req.Uid = s.Data.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		s.Reply(ret.(*protoActivity.GetProgressAck))
	})
}

func ClaimReward(s *Session, msg interface{}) {
	req := msg.(*protoActivity.ClaimRewardReq)
	req.Uid = s.Data.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		s.Reply(ret.(*protoActivity.ClaimRewardAck))
	})
}

func requestActivity(s *Session, msg interface{ ProtoMessage() }, cb func(interface{}, error)) {
	s.Request(actor.MustLookup(constant.ActorActivity), msg, func(ret interface{}, err error) {
		if err != nil {
			log.Printf("requestActivity error: %v", err)
		}
		cb(ret, err)
	})
}

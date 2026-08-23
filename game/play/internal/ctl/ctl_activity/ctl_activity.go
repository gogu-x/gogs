package ctl_activity

import (
	"log"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

func GetActivityList(s *base.PlayContext, req *protoActivity.GetActivityListReq) {
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*protoActivity.GetActivityListAck); ok {
			s.Reply(ack)
		}
	})
}

func JoinActivity(s *base.PlayContext, req *protoActivity.JoinActivityReq) {
	req.Uid = s.Player.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*protoActivity.JoinActivityAck); ok {
			s.Reply(ack)
		}
	})
}

func GetProgress(s *base.PlayContext, req *protoActivity.GetProgressReq) {
	req.Uid = s.Player.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*protoActivity.GetProgressAck); ok {
			s.Reply(ack)
		}
	})
}

func ClaimReward(s *base.PlayContext, req *protoActivity.ClaimRewardReq) {
	req.Uid = s.Player.UID
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*protoActivity.ClaimRewardAck); ok {
			s.Reply(ack)
		}
	})
}

func requestActivity(s *base.PlayContext, msg proto.Message, cb func(interface{}, error)) {
	pid, ok := s.Tree().Lookup(constant.ActorActivity)
	if !ok {
		log.Printf("play: activity actor not found")
		return
	}
	s.Request(pid, msg, func(_ tree.Context, ret interface{}, err error) {
		if err != nil {
			log.Printf("play: request activity error: %v", err)
			return
		}
		cb(ret, err)
	})
}

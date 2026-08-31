package ctl_activity

import (
	"log"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/gogs/pb/pb_activity"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

func GetActivityList(s *common.PlayerContext, req *pb_activity.GetActivityListReq) {
	requestActivity(s, req,
		func(ret interface{}, err error) {
			if ack, ok := ret.(*pb_activity.GetActivityListAck); ok {
				s.Reply(ack)
			}
		})
}

func JoinActivity(s *common.PlayerContext, req *pb_activity.JoinActivityReq) {
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*pb_activity.JoinActivityAck); ok {
			s.Reply(ack)
		}
	})
}

func GetProgress(s *common.PlayerContext, req *pb_activity.GetProgressReq) {
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*pb_activity.GetProgressAck); ok {
			s.Reply(ack)
		}
	})
}

func ClaimReward(s *common.PlayerContext, req *pb_activity.ClaimRewardReq) {
	requestActivity(s, req, func(ret interface{}, err error) {
		if ack, ok := ret.(*pb_activity.ClaimRewardAck); ok {
			s.Reply(ack)
		}
	})
}

func requestActivity(s *common.PlayerContext, msg proto.Message, cb func(interface{}, error)) {
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

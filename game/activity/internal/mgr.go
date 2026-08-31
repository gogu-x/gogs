package internal

import (
	"github.com/gogu-x/gogs/pb/pb_activity"
	"github.com/gogu-x/gogs/pb/pb_common"
	"github.com/gogu-x/tree"
)

type Activity struct {
	ID        uint64
	Name      string
	Status    pb_activity.ActivityStatus
	StartTime int64
	EndTime   int64
}

func (a *Activity) ToProto() *pb_activity.ActivityInfo {
	return &pb_activity.ActivityInfo{
		Id: a.ID, Name: a.Name, Status: a.Status,
		StartTime: a.StartTime, EndTime: a.EndTime,
	}
}

type Progress struct {
	ActivityID uint64
	UID        uint64
	Progress   int32
	Target     int32
	Rewarded   bool
}

func (p *Progress) ToProto() *pb_activity.ActivityProgress {
	return &pb_activity.ActivityProgress{
		ActivityId: p.ActivityID, Uid: p.UID,
		Progress: p.Progress, Target: p.Target, Rewarded: p.Rewarded,
	}
}

type Mgr struct {
	activities map[uint64]*Activity
	progresses map[uint64]map[uint64]*Progress
}

func NewMgr() *Mgr {
	return &Mgr{
		activities: make(map[uint64]*Activity),
		progresses: make(map[uint64]map[uint64]*Progress),
	}
}

func (s *Mgr) GetList(ctx tree.Context, _ *pb_activity.GetActivityListReq) {
	list := make([]*pb_activity.ActivityInfo, 0, len(s.activities))
	for _, a := range s.activities {
		if a.Status == pb_activity.ActivityStatus_ONGOING {
			list = append(list, a.ToProto())
		}
	}
	ctx.Response(&pb_activity.GetActivityListAck{Activities: list}, nil)
}

func (s *Mgr) Join(ctx tree.Context, req *pb_activity.JoinActivityReq) {
	ack := &pb_activity.JoinActivityAck{}
	defer ctx.Response(ack, nil)
	act, ok := s.activities[req.ActivityId]
	if !ok || act.Status != pb_activity.ActivityStatus_ONGOING {
		ack.Code = pb_common.ErrCode_ERR_UNKNOWN
		ack.Msg = "activity not available"
		return
	}
	if _, exists := s.getProgress(req.GetUID(), req.ActivityId); exists {
		ack.Code = pb_common.ErrCode_ERR_ALREADY_IN_GUILD
		ack.Msg = "already joined"
		return
	}
	s.setProgress(req.GetUID(), &Progress{ActivityID: req.ActivityId, UID: req.GetUID(), Target: 100})
}

func (s *Mgr) GetProgress(ctx tree.Context, req *pb_activity.GetProgressReq) {
	ack := &pb_activity.GetProgressAck{Code: pb_common.ErrCode_ERR_UNKNOWN}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.GetUID(), req.ActivityId)
	if !ok {
		return
	}
	ack.Code = pb_common.ErrCode_OK
	ack.Progress = p.ToProto()
}

func (s *Mgr) ClaimReward(ctx tree.Context, req *pb_activity.ClaimRewardReq) {
	ack := &pb_activity.ClaimRewardAck{}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.GetUID(), req.ActivityId)
	if !ok {
		ack.Code = pb_common.ErrCode_ERR_UNKNOWN
		ack.Msg = "not joined"
		return
	}
	if p.Rewarded {
		ack.Code = pb_common.ErrCode_ERR_UNKNOWN
		ack.Msg = "already claimed"
		return
	}
	if p.Progress < p.Target {
		ack.Code = pb_common.ErrCode_ERR_UNKNOWN
		ack.Msg = "not completed"
		return
	}
	p.Rewarded = true
}

func (s *Mgr) getProgress(uid, activityID uint64) (*Progress, bool) {
	if m, ok := s.progresses[uid]; ok {
		p, ok := m[activityID]
		return p, ok
	}
	return nil, false
}

func (s *Mgr) setProgress(uid uint64, p *Progress) {
	if s.progresses[uid] == nil {
		s.progresses[uid] = make(map[uint64]*Progress)
	}
	s.progresses[uid][p.ActivityID] = p
}

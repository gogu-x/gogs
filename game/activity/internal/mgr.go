package internal

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoActivity"
	"github.com/gogu-x/gogs/pb/protoCommon"
)

type Activity struct {
	ID        uint64
	Name      string
	Status    protoActivity.ActivityStatus
	StartTime int64
	EndTime   int64
}

func (a *Activity) ToProto() *protoActivity.ActivityInfo {
	return &protoActivity.ActivityInfo{
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

func (p *Progress) ToProto() *protoActivity.ActivityProgress {
	return &protoActivity.ActivityProgress{
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

func (s *Mgr) GetList(ctx actor.ActorContext, _ *protoActivity.GetActivityListReq) {
	list := make([]*protoActivity.ActivityInfo, 0, len(s.activities))
	for _, a := range s.activities {
		if a.Status == protoActivity.ActivityStatus_ONGOING {
			list = append(list, a.ToProto())
		}
	}
	ctx.Response(&protoActivity.GetActivityListAck{Activities: list}, nil)
}

func (s *Mgr) Join(ctx actor.ActorContext, req *protoActivity.JoinActivityReq) {
	ack := &protoActivity.JoinActivityAck{}
	defer ctx.Response(ack, nil)
	act, ok := s.activities[req.ActivityId]
	if !ok || act.Status != protoActivity.ActivityStatus_ONGOING {
		ack.Code = protoCommon.ErrCode_ERR_UNKNOWN
		ack.Msg = "activity not available"
		return
	}
	if _, exists := s.getProgress(req.Uid, req.ActivityId); exists {
		ack.Code = protoCommon.ErrCode_ERR_ALREADY_IN_GUILD
		ack.Msg = "already joined"
		return
	}
	s.setProgress(req.Uid, &Progress{ActivityID: req.ActivityId, UID: req.Uid, Target: 100})
}

func (s *Mgr) GetProgress(ctx actor.ActorContext, req *protoActivity.GetProgressReq) {
	ack := &protoActivity.GetProgressAck{Code: protoCommon.ErrCode_ERR_UNKNOWN}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		return
	}
	ack.Code = protoCommon.ErrCode_OK
	ack.Progress = p.ToProto()
}

func (s *Mgr) ClaimReward(ctx actor.ActorContext, req *protoActivity.ClaimRewardReq) {
	ack := &protoActivity.ClaimRewardAck{}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		ack.Code = protoCommon.ErrCode_ERR_UNKNOWN
		ack.Msg = "not joined"
		return
	}
	if p.Rewarded {
		ack.Code = protoCommon.ErrCode_ERR_UNKNOWN
		ack.Msg = "already claimed"
		return
	}
	if p.Progress < p.Target {
		ack.Code = protoCommon.ErrCode_ERR_UNKNOWN
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

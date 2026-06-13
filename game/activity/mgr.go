package activity

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/pb/protoActivity"
)

// Activity 活动数据
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

// Progress 玩家在某活动的进度
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

// ActivityMgr 活动数据存储，纯数据操作，在 ActivityActor 单 goroutine 内使用
type ActivityMgr struct {
	activities map[uint64]*Activity            // activityID → Activity
	progresses map[uint64]map[uint64]*Progress // uid → activityID → Progress
}

func NewActivityMgr() *ActivityMgr {
	return &ActivityMgr{
		activities: make(map[uint64]*Activity),
		progresses: make(map[uint64]map[uint64]*Progress),
	}
}

// AddActivity 添加活动（由 GM 或定时任务调用）
func (s *ActivityMgr) AddActivity(a *Activity) {
	s.activities[a.ID] = a
}

func (s *ActivityMgr) GetList(ctx actor.ActorContext, _ *protoActivity.GetActivityListReq) {
	list := make([]*protoActivity.ActivityInfo, 0, len(s.activities))
	for _, a := range s.activities {
		if a.Status == protoActivity.ActivityStatus_ONGOING {
			list = append(list, a.ToProto())
		}
	}
	ctx.Response(&protoActivity.GetActivityListResp{Activities: list}, nil)
}

func (s *ActivityMgr) Join(ctx actor.ActorContext, req *protoActivity.JoinActivityReq) {
	ack := &protoActivity.JoinActivityResp{}
	defer ctx.Response(ack, nil)
	act, ok := s.activities[req.ActivityId]
	if !ok || act.Status != protoActivity.ActivityStatus_ONGOING {
		ack.Code = 1
		ack.Msg = "activity not available"
		return
	}
	if _, exists := s.getProgress(req.Uid, req.ActivityId); exists {
		ack.Code = 2
		ack.Msg = "already joined"
		return
	}
	s.setProgress(req.Uid, &Progress{
		ActivityID: req.ActivityId, UID: req.Uid, Target: 100,
	})
}

func (s *ActivityMgr) GetProgress(ctx actor.ActorContext, req *protoActivity.GetProgressReq) {
	ack := &protoActivity.GetProgressResp{Code: 1}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		return
	}
	ack.Code = 0
	ack.Progress = p.ToProto()
}

func (s *ActivityMgr) ClaimReward(ctx actor.ActorContext, req *protoActivity.ClaimRewardReq) {
	ack := &protoActivity.ClaimRewardResp{}
	defer ctx.Response(ack, nil)
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		ack.Code = 1
		ack.Msg = "not joined"
		return
	}
	if p.Rewarded {
		ack.Code = 2
		ack.Msg = "already claimed"
		return
	}
	if p.Progress < p.Target {
		ack.Code = 3
		ack.Msg = "not completed"
		return
	}
	p.Rewarded = true
}

func (s *ActivityMgr) getProgress(uid, activityID uint64) (*Progress, bool) {
	if m, ok := s.progresses[uid]; ok {
		p, ok := m[activityID]
		return p, ok
	}
	return nil, false
}

func (s *ActivityMgr) setProgress(uid uint64, p *Progress) {
	if s.progresses[uid] == nil {
		s.progresses[uid] = make(map[uint64]*Progress)
	}
	s.progresses[uid][p.ActivityID] = p
}

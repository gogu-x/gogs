package activity

import (
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

func (s *ActivityMgr) GetList(_ *protoActivity.GetActivityListReq) *protoActivity.GetActivityListResp {
	list := make([]*protoActivity.ActivityInfo, 0, len(s.activities))
	for _, a := range s.activities {
		if a.Status == protoActivity.ActivityStatus_ONGOING {
			list = append(list, a.ToProto())
		}
	}
	return &protoActivity.GetActivityListResp{Activities: list}
}

func (s *ActivityMgr) Join(req *protoActivity.JoinActivityReq) *protoActivity.JoinActivityResp {
	act, ok := s.activities[req.ActivityId]
	if !ok || act.Status != protoActivity.ActivityStatus_ONGOING {
		return &protoActivity.JoinActivityResp{Code: 1, Msg: "activity not available"}
	}
	if _, exists := s.getProgress(req.Uid, req.ActivityId); exists {
		return &protoActivity.JoinActivityResp{Code: 2, Msg: "already joined"}
	}
	s.setProgress(req.Uid, &Progress{
		ActivityID: req.ActivityId, UID: req.Uid, Target: 100,
	})
	return &protoActivity.JoinActivityResp{}
}

func (s *ActivityMgr) GetProgress(req *protoActivity.GetProgressReq) *protoActivity.GetProgressResp {
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		return &protoActivity.GetProgressResp{Code: 1}
	}
	return &protoActivity.GetProgressResp{Progress: p.ToProto()}
}

func (s *ActivityMgr) ClaimReward(req *protoActivity.ClaimRewardReq) *protoActivity.ClaimRewardResp {
	p, ok := s.getProgress(req.Uid, req.ActivityId)
	if !ok {
		return &protoActivity.ClaimRewardResp{Code: 1, Msg: "not joined"}
	}
	if p.Rewarded {
		return &protoActivity.ClaimRewardResp{Code: 2, Msg: "already claimed"}
	}
	if p.Progress < p.Target {
		return &protoActivity.ClaimRewardResp{Code: 3, Msg: "not completed"}
	}
	p.Rewarded = true
	return &protoActivity.ClaimRewardResp{}
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

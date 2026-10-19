package protocol

import (
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
)

// ToInfo 将活动实例转换为客户端活动信息。
func ToInfo(instance *activitymodel.Instance) *pb_activity.ActivityInfo {
	if instance == nil {
		return nil
	}
	return &pb_activity.ActivityInfo{
		Id:        instance.GetID(),
		Name:      instance.GetName(),
		Status:    ToStatus(instance.GetStatus()),
		StartTime: instance.GetStartTime(),
		EndTime:   instance.GetEndTime(),
	}
}

// ToProgress 将活动实例转换为客户端进度信息。
func ToProgress(instance *activitymodel.Instance) *pb_activity.ActivityProgress {
	if instance == nil {
		return nil
	}
	return &pb_activity.ActivityProgress{
		ActivityId: instance.GetID(),
		Uid:        instance.GetOwnerID(),
		Progress:   instance.GetProgress(),
		Target:     instance.GetTarget(),
		Rewarded:   instance.IsRewarded(),
	}
}

// ToStatus 将服务端生命周期状态转换为客户端活动状态。
func ToStatus(status activitymodel.Status) pb_activity.ActivityStatus {
	switch status {
	case activitymodel.StatusScheduled:
		return pb_activity.ActivityStatus_NOT_STARTED
	case activitymodel.StatusStarting:
		return pb_activity.ActivityStatus_ONGOING
	case activitymodel.StatusRunning:
		return pb_activity.ActivityStatus_ONGOING
	case activitymodel.StatusClosing:
		return pb_activity.ActivityStatus_ENDED
	case activitymodel.StatusClosed:
		return pb_activity.ActivityStatus_ENDED
	default:
		return pb_activity.ActivityStatus_NOT_STARTED
	}
}

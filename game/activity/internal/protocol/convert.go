package protocol

import (
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
)

// ToInfo converts an activity instance to the existing client activity message.
func ToInfo(instance *activitymodel.Instance) *pb_activity.ActivityInfo {
	if instance == nil {
		return nil
	}
	return &pb_activity.ActivityInfo{
		Id:        instance.GetTemplateID(),
		Name:      instance.GetName(),
		Status:    ToStatus(instance.GetStatus()),
		StartTime: instance.GetStartTime(),
		EndTime:   instance.GetEndTime(),
	}
}

// ToProgress converts a player activity instance to the existing progress message.
func ToProgress(instance *activitymodel.Instance) *pb_activity.ActivityProgress {
	if instance == nil {
		return nil
	}
	return &pb_activity.ActivityProgress{
		ActivityId: instance.GetTemplateID(),
		Uid:        instance.GetOwnerID(),
		Progress:   instance.GetProgress(),
		Target:     instance.GetTarget(),
		Rewarded:   instance.IsRewarded(),
	}
}

// ToStatus maps the detailed server lifecycle into the compatible client enum.
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

package service

import (
	"time"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

// ClientRequestHandler reports an explicit outcome for implemented client operations.
type ClientRequestHandler interface {
	HandleClientRequest(uint64, proto.Message) pb_common.ErrCode
}

func (as *ActvService) HandleClientRequest(uid uint64, activityID int64, request proto.Message) pb_common.ErrCode {
	if uid == 0 || request == nil || activityID <= 0 {
		return pb_common.ErrCode_PARAM
	}
	if as == nil {
		return pb_common.ErrCode_ACTIVITY_UNAVAILABLE
	}
	actv := as.GetActvByID(activityID)
	if actv == nil || actv.GetMeta() == nil {
		return pb_common.ErrCode_ACTIVITY_NOT_FOUND
	}
	meta := actv.GetMeta()
	if meta.GetOwnerScope() == ScopePlayer && meta.GetOwnerID() != OwnerID(uid) {
		return pb_common.ErrCode_ACTIVITY_NOT_FOUND
	}
	now := time.Now().Unix()
	if actv.IsEnded() || meta.IsExpired(now) || meta.GetStartTs() > now {
		return pb_common.ErrCode_ACTIVITY_NOT_OPEN
	}
	handler, ok := actv.GetExt().(ClientRequestHandler)
	if !ok {
		return pb_common.ErrCode_ACTIVITY_UNAVAILABLE
	}
	return handler.HandleClientRequest(uid, request)
}

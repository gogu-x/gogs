package online

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
)

// HandleClientRequest acknowledges only implemented template operations.
func (b *Behavior) HandleClientRequest(uid uint64, request proto.Message) pb_common.ErrCode {
	join, ok := request.(*pb_activity.JoinActivityReq)
	if !ok || join == nil {
		return pb_common.ErrCode_ACTIVITY_UNAVAILABLE
	}
	if b == nil || b.member == nil {
		return pb_common.ErrCode_ACTIVITY_UNAVAILABLE
	}
	if b.member.Get(uid) == nil {
		return pb_common.ErrCode_ACTIVITY_UNAVAILABLE
	}
	b.handleJoinActivity(uid, join)
	return pb_common.ErrCode_OK
}

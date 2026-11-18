package sevenday

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"time"
)

// handleGetSevenDayLogin 提供可选补拉接口，所有业务校验及响应由模板决定。
// @actv.request
func (b *Behavior) handleGetSevenDayLogin(uid uint64, request *pb_actv.GetSevenDayLoginReq) {
	meta := b.Meta()
	ack := &pb_actv.GetSevenDayLoginAck{Code: pb_common.ErrCode_OK}
	switch {
	case uid == 0 || uint64(meta.GetOwnerID()) != uid || request.GetActvId() != uint64(meta.GetActvID()):
		ack.Code = pb_common.ErrCode_ACTV_NOT_FOUND
	case b.login.Blocked():
		ack.Code = pb_common.ErrCode_ACTV_UNAVAILABLE
	case meta.GetStartTs() > time.Now().Unix():
		ack.Code = pb_common.ErrCode_ACTV_NOT_OPEN
	default:
		b.advance(time.Now(), b.online)
		ack.Data = b.packState()
	}
	b.PushToPlayer(uid, ack)
}

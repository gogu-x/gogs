package ctl_actv

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// OnGetActivityList 将玩家活动列表请求转发到全局活动 Actor。
func OnGetActivityList(ctx *core.Context, request *pb_activity.GetActivityListReq) {
	if request == nil {
		return
	}
	forwardActivityRequest(ctx, request.GetUID(), request, &pb_activity.GetActivityListAck{})
}

// OnJoinActivity 将玩家加入活动请求转发到全局活动 Actor。
func OnJoinActivity(ctx *core.Context, request *pb_activity.JoinActivityReq) {
	if request == nil {
		return
	}
	forwardActivityRequest(ctx, request.GetUID(), request, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_INTERNAL})
}

// OnGetActivityProgress 将玩家活动进度请求转发到全局活动 Actor。
func OnGetActivityProgress(ctx *core.Context, request *pb_activity.GetProgressReq) {
	if request == nil {
		return
	}
	forwardActivityRequest(ctx, request.GetUID(), request, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_INTERNAL})
}

// OnClaimActivityReward 将玩家领取活动奖励请求转发到全局活动 Actor。
func OnClaimActivityReward(ctx *core.Context, request *pb_activity.ClaimRewardReq) {
	if request == nil {
		return
	}
	forwardActivityRequest(ctx, request.GetUID(), request, &pb_activity.ClaimRewardAck{Code: pb_common.ErrCode_INTERNAL})
}

// forwardActivityRequest 通过玩家 Actor 转发活动请求并将结果推送回客户端。
func forwardActivityRequest(
	ctx *core.Context,
	uid uint64,
	message interface{},
	fallback proto.Message,
) {
	if ctx == nil || uid == 0 || message == nil {
		return
	}
	activityMessage := &ipb.PlayerActivityMessage{
		UID:        uid,
		Generation: ctx.SessionGeneration(),
		Online:     ctx.SessionActive(),
		Message:    message,
	}
	if !ctx.CastCall(def.ActorActivity, activityMessage, func(callbackCtx tree.Context, value interface{}, err error) {
		response, valid := value.(proto.Message)
		if err != nil || !valid || response == nil {
			tlog.Log.Error("[play/ctl_actv.forwardActivityRequest] 活动请求处理失败, pid=%v type=%T err=%v", uid, message, err)
			response = fallback
		}
		pushActivityResponse(callbackCtx, uid, response)
	}) {
		tlog.Log.Error("[play/ctl_actv.forwardActivityRequest] 全局活动 Actor 不可用, pid=%v type=%T", uid, message)
		pushActivityResponse(ctx.TreeCtx, uid, fallback)
	}
}

// pushActivityResponse 通过 Gate 将活动响应送回对应玩家。
func pushActivityResponse(ctx tree.Context, uid uint64, message proto.Message) {
	if ctx == nil || uid == 0 || message == nil {
		return
	}
	gatePID, ok := ctx.Lookup(def.GameGate)
	if !ok || !ctx.Send(gatePID, &ipb.PushToMsg{UID: uid, Msg: message}) {
		tlog.Log.Error("[play/ctl_actv.pushActivityResponse] 活动响应推送失败, pid=%v", uid)
	}
}

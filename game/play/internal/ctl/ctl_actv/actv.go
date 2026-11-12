package ctl_actv

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
)

// OnGetActvList 将玩家活动列表请求转发到全局活动 Actor。
func OnGetActvList(ctx *core.Context, request *pb_actv.GetActvListReq) {
	if request == nil {
		return
	}
	forwardActvRequest(ctx, 0, request)
}

// OnGetActvProgress 将玩家活动进度请求转发到全局活动 Actor。
func OnGetActvProgress(ctx *core.Context, request *pb_actv.GetProgressReq) {
	if request == nil {
		return
	}
	forwardActvRequest(ctx, request.GetActvId(), request)
}

// OnClaimActvReward 将玩家领取活动奖励请求转发到全局活动 Actor。
func OnClaimActvReward(ctx *core.Context, request *pb_actv.ClaimRewardReq) {
	if request == nil {
		return
	}
	forwardActvRequest(ctx, request.GetActvId(), request)
}

// OnGetSevenDayLogin 将七日登录补拉请求单向转发给活动模板。
func OnGetSevenDayLogin(ctx *core.Context, request *pb_actv.GetSevenDayLoginReq) {
	if request == nil {
		return
	}
	forwardActvRequest(ctx, request.GetActvId(), request)
}

// OnGetTaskLottery 将玩法请求单向投递给活动模板。
func OnGetTaskLottery(ctx *core.Context, request *pb_actv.GetTaskLotteryReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

// OnClaimActvTask 将玩法请求单向投递给活动模板。
func OnClaimActvTask(ctx *core.Context, request *pb_actv.ClaimActvTaskReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

// OnDrawActv 将玩法请求单向投递给活动模板。
func OnDrawActv(ctx *core.Context, request *pb_actv.DrawActvReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

// OnBuyActvShop 将玩法请求单向投递给活动模板。
func OnBuyActvShop(ctx *core.Context, request *pb_actv.BuyActvShopReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

// OnGetActvShop 转发独立商店查询。
func OnGetActvShop(ctx *core.Context, request *pb_actv.GetActvShopReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

// OnRefreshActvShop 转发商店每日刷新请求。
func OnRefreshActvShop(ctx *core.Context, request *pb_actv.RefreshActvShopReq) {
	if request != nil {
		forwardActvRequest(ctx, request.GetActvId(), request)
	}
}

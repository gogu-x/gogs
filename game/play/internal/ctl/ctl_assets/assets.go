package ctl_assets

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player/asset"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/tlog"
)

// OnActvAssetTrade 通过统一ctl业务路由处理活动资产消息，数据加载由玩家公共路由负责。
func OnActvAssetTrade(ctx *core.Context, tx *ipb.ActvAssetTrade) {
	if tx == nil {
		return
	}
	code := pb_common.ErrCode_ACTV_UNAVAILABLE
	uid := ctx.PlayerID()
	if current := ctx.GetPlayer(); current != nil && tx.GetUID() == uid {
		code = asset.ApplyAssetTrade(current, tx)
		if code == pb_common.ErrCode_OK {
			ctx.CastPlayerMsg(uid, current.GetAssetMgr().Message())
		}
	}
	ctx.Response(code, nil)

	tlog.Log.Info("[ctl_assets/OnActvAssetTrade] 资源处理完成, pid=%v actv=%v sequence=%v code=%v", uid, tx.GetActvID(), tx.GetSequence(), code)
}

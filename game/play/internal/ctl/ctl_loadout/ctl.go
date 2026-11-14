package ctl_loadout

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree/tlog"
)

// GetSoulJade 接收客户端请求并返回模块处理结果。
func GetSoulJade(ctx *core.Context, req *pb_souljade.GetRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.GetRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/GetSoulJade] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data = souljade.BuildData(p)
	if data := ack.GetData(); data == nil || data.GetRoleId() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
}

// ChangeSoulJade 接收客户端请求并返回模块处理结果。
func ChangeSoulJade(ctx *core.Context, req *pb_souljade.ChangeRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.ChangeRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data, ack.Code = souljade.ChangeSoulJade(p, req)
}

// UpgradeSoulJade 接收客户端请求并返回模块处理结果。
func UpgradeSoulJade(ctx *core.Context, req *pb_souljade.UpgradeRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.UpgradeRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data, ack.Code = souljade.UpgradeSoulJade(p, req)
}

// RefineSoulJade 接收客户端请求并返回模块处理结果。
func RefineSoulJade(ctx *core.Context, req *pb_souljade.RefineRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.RefineRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data, ack.Code = souljade.RefineSoulJade(p, req)
}

// GetTalent 接收客户端请求并返回模块处理结果。
func GetTalent(ctx *core.Context, req *pb_talent.GetRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.GetRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/GetTalent] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data = talent.BuildData(p)
	if data := ack.GetData(); data == nil || data.GetRoleId() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
}

// LearnTalent 接收客户端请求并返回模块处理结果。
func LearnTalent(ctx *core.Context, req *pb_talent.LearnRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.LearnRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/LearnTalent] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data, ack.Code = talent.LearnTalent(p, req)
}

// ResetTalent 接收客户端请求并返回模块处理结果。
func ResetTalent(ctx *core.Context, req *pb_talent.ResetRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.ResetRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer func() {
		ctx.CastPlayerMsg(uid, ack)
		tlog.Log.Info("[ctl_loadout/ResetTalent] 请求处理完成, request=%v, code=%v, pid=%v",
			ack.GetRequestId(), ack.GetCode(), uid)
	}()
	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	ack.RequestId = req.GetRequestId()
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		return
	}
	ack.Data, ack.Code = talent.ResetTalent(p, req)
}

package ctl_loadout

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
	"github.com/gogu-x/gogs/pb/cspb/pb_talent"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// reply 推送响应并记录处理结果；请求与玩家数据已由 core.RegisterPlayerMsg 校验。
func reply(ctx *core.Context, name string, requestID uint64, code pb_common.ErrCode, ack proto.Message) {
	uid := ctx.PlayerID()
	ctx.CastPlayerMsg(uid, ack)
	tlog.Log.Info("[ctl_loadout/%v] 请求处理完成, request=%v, code=%v, pid=%v", name, requestID, code, uid)
}

// GetSoulJade 返回玩家魂玉数据；玩家没有魂玉数据时返回空数据。
func GetSoulJade(ctx *core.Context, req *pb_souljade.GetRoleSoulJadeReq) {
	ack := &pb_souljade.GetRoleSoulJadeAck{RequestId: req.GetRequestId(), Code: pb_common.ErrCode_OK, Data: souljade.BuildData(ctx.GetPlayer())}
	reply(ctx, "GetSoulJade", ack.GetRequestId(), ack.GetCode(), ack)
}

// ChangeSoulJade 更换或卸下魂玉并返回最新数据。
func ChangeSoulJade(ctx *core.Context, req *pb_souljade.ChangeRoleSoulJadeReq) {
	ack := &pb_souljade.ChangeRoleSoulJadeAck{RequestId: req.GetRequestId()}
	ack.Data, ack.Code = souljade.ChangeSoulJade(ctx.GetPlayer(), req)
	reply(ctx, "ChangeSoulJade", ack.GetRequestId(), ack.GetCode(), ack)
}

// UpgradeSoulJade 升级魂玉并返回最新数据。
func UpgradeSoulJade(ctx *core.Context, req *pb_souljade.UpgradeRoleSoulJadeReq) {
	ack := &pb_souljade.UpgradeRoleSoulJadeAck{RequestId: req.GetRequestId()}
	ack.Data, ack.Code = souljade.UpgradeSoulJade(ctx.GetPlayer(), req)
	reply(ctx, "UpgradeSoulJade", ack.GetRequestId(), ack.GetCode(), ack)
}

// RefineSoulJade 洗练魂玉并返回最新数据。
func RefineSoulJade(ctx *core.Context, req *pb_souljade.RefineRoleSoulJadeReq) {
	ack := &pb_souljade.RefineRoleSoulJadeAck{RequestId: req.GetRequestId()}
	ack.Data, ack.Code = souljade.RefineSoulJade(ctx.GetPlayer(), req)
	reply(ctx, "RefineSoulJade", ack.GetRequestId(), ack.GetCode(), ack)
}

// GetTalent 返回玩家天赋数据；玩家没有天赋数据时返回空数据。
func GetTalent(ctx *core.Context, req *pb_talent.GetRoleTalentReq) {
	ack := &pb_talent.GetRoleTalentAck{RequestId: req.GetRequestId(), Code: pb_common.ErrCode_OK, Data: talent.BuildData(ctx.GetPlayer())}
	reply(ctx, "GetTalent", ack.GetRequestId(), ack.GetCode(), ack)
}

// LearnTalent 学习天赋节点并返回最新数据。
func LearnTalent(ctx *core.Context, req *pb_talent.LearnRoleTalentReq) {
	ack := &pb_talent.LearnRoleTalentAck{RequestId: req.GetRequestId()}
	ack.Data, ack.Code = talent.LearnTalent(ctx.GetPlayer(), req)
	reply(ctx, "LearnTalent", ack.GetRequestId(), ack.GetCode(), ack)
}

// ResetTalent 重置天赋并返回最新数据。
func ResetTalent(ctx *core.Context, req *pb_talent.ResetRoleTalentReq) {
	ack := &pb_talent.ResetRoleTalentAck{RequestId: req.GetRequestId()}
	ack.Data, ack.Code = talent.ResetTalent(ctx.GetPlayer(), req)
	reply(ctx, "ResetTalent", ack.GetRequestId(), ack.GetCode(), ack)
}

package ctl_equipment

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/tree/tlog"
)

// OnGetRoleEquipment returns the authoritative equipment snapshot.
func OnGetRoleEquipment(ctx *core.Context, req *pb_equipment.GetRoleEquipmentReq) {
	p := ctx.GetPlayer()
	uid := ctx.PlayerID()
	data, code := equipment.BuildRoleEquipmentData(p.GetRoleMgr(), p.GetEquipmentMgr())
	ack := &pb_equipment.GetRoleEquipmentAck{Data: data, RequestId: req.GetRequestId(), Code: pb_common.ErrCode_OK}
	if code != pb_common.ErrCode_OK {
		ack.Code = pb_common.ErrCode_EQUIPMENT_UNAVAILABLE
		tlog.Log.Error("[ctl_equipment/OnGetRoleEquipment] 组装装备视图失败, pid=%v code=%v", uid, code)
	}
	ctx.CastPlayerMsg(uid, ack)
}

// OnChangeRoleEquipment applies a validated equipment change.
func OnChangeRoleEquipment(ctx *core.Context, req *pb_equipment.ChangeRoleEquipmentReq) {
	p := ctx.GetPlayer()
	uid := ctx.PlayerID()
	slot := int32(req.GetSlot())
	equipmentID := req.GetEquipmentId()
	roleMgr := p.GetRoleMgr()
	equipmentMgr := p.GetEquipmentMgr()
	character, hasCharacter := roleMgr.GetCharacter()
	var beforeIDs []string
	if hasCharacter {
		beforeIDs = character.GetEquipmentIDs()
	}
	code := equipment.ChangeRoleEquipment(roleMgr, equipmentMgr, slot, equipmentID)
	ack := &pb_equipment.ChangeRoleEquipmentAck{RequestId: req.GetRequestId(), Code: pb_common.ErrCode_OK}
	if code != pb_common.ErrCode_OK {
		ack.Code = pb_common.ErrCode_INVALID_EQUIPMENT
		tlog.Log.Error("[ctl_equipment/OnChangeRoleEquipment] 装备变更校验失败, pid=%v slot=%v equipment=%v code=%v", uid, slot, equipmentID, code)
		ctx.CastPlayerMsg(uid, ack)
		return
	}
	p.MarkDirty()
	data, code := equipment.BuildRoleEquipmentData(roleMgr, equipmentMgr)
	if code != pb_common.ErrCode_OK {
		ack.Code = pb_common.ErrCode_EQUIPMENT_UNAVAILABLE
		tlog.Log.Error("[ctl_equipment/OnChangeRoleEquipment] 装备视图刷新失败, pid=%v code=%v", uid, code)
		ctx.CastPlayerMsg(uid, ack)
		return
	}
	ack.Data = data
	character, hasCharacter = roleMgr.GetCharacter()
	var afterIDs []string
	if hasCharacter {
		afterIDs = character.GetEquipmentIDs()
	}
	tlog.Log.Info("[ctl_equipment/OnChangeRoleEquipment] 玩家装备已修改, pid=%v slot=%v before=%v after=%v",
		uid, slot, beforeIDs, afterIDs)
	ctx.CastPlayerMsg(uid, ack)
}

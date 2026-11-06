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

// GetSoulJade returns an explicit status and the current authoritative snapshot.
func GetSoulJade(ctx *core.Context, req *pb_souljade.GetRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.GetRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/GetSoulJade] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	ack.RequestId = requestID
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetSoulJade] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	data := p.BuildSoulJadeData()
	ack.Data = data
	if data == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetSoulJade] 养成快照不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if data.GetRoleId() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetSoulJade] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	tlog.Log.Info("[ctl_loadout/GetSoulJade] 返回魂玉数据, request=%v, code=%v, pid=%v",
		requestID, ack.GetCode(), uid)
}

// GetTalent returns an explicit status and the current authoritative snapshot.
func GetTalent(ctx *core.Context, req *pb_talent.GetRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.GetRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/GetTalent] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	ack.RequestId = requestID
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetTalent] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	data := p.BuildTalentData()
	ack.Data = data
	if data == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetTalent] 养成快照不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if data.GetRoleId() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/GetTalent] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	tlog.Log.Info("[ctl_loadout/GetTalent] 返回天赋数据, request=%v, code=%v, pid=%v",
		requestID, ack.GetCode(), uid)
}

// ChangeSoulJade validates and changes one configured slot.
func ChangeSoulJade(ctx *core.Context, req *pb_souljade.ChangeRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.ChangeRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	expected := req.GetExpectedRevision()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 收到养成操作, request=%v, expected=%v, pid=%v",
		requestID, expected, uid)

	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	ack.Data = p.BuildSoulJadeData()
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.GetCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character.GetID() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	mgr := p.GetSoulJadeMgr()
	if mgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 养成模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	before := mgr.GetRevision()
	if before == 0 {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 养成版本尚未初始化, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if requestID == 0 {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 请求编号无效, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if expected != before {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 养成版本已变化, request=%v, expected=%v, before=%v, code=%v, pid=%v",
			requestID, expected, before, ack.GetCode(), uid)
		return
	}

	code := souljade.Change(mgr, req.GetSlot(), req.GetInstanceId())
	ack.Code = code
	if code != pb_common.ErrCode_OK {
		tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 养成操作被拒绝, request=%v, code=%v, before=%v, after=%v, pid=%v",
			requestID, code, before, mgr.GetRevision(), uid)
		return
	}
	p.MarkDirty()
	ack.Data = p.BuildSoulJadeData()
	tlog.Log.Info("[ctl_loadout/ChangeSoulJade] 养成操作已处理, request=%v, code=%v, before=%v, after=%v, pid=%v",
		requestID, code, before, mgr.GetRevision(), uid)
}

// UpgradeSoulJade checks the request revision before spending upgrade materials.
func UpgradeSoulJade(ctx *core.Context, req *pb_souljade.UpgradeRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.UpgradeRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	expected := req.GetExpectedRevision()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 收到养成操作, request=%v, expected=%v, pid=%v",
		requestID, expected, uid)

	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	ack.Data = p.BuildSoulJadeData()
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.GetCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character.GetID() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	mgr := p.GetSoulJadeMgr()
	if mgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 养成模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	before := mgr.GetRevision()
	if before == 0 {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 养成版本尚未初始化, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if requestID == 0 {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 请求编号无效, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if expected != before {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 养成版本已变化, request=%v, expected=%v, before=%v, code=%v, pid=%v",
			requestID, expected, before, ack.GetCode(), uid)
		return
	}

	code := souljade.Upgrade(mgr, req.GetInstanceId())
	ack.Code = code
	if code != pb_common.ErrCode_OK {
		tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 养成操作被拒绝, request=%v, code=%v, before=%v, after=%v, pid=%v",
			requestID, code, before, mgr.GetRevision(), uid)
		return
	}
	p.MarkDirty()
	ack.Data = p.BuildSoulJadeData()
	tlog.Log.Info("[ctl_loadout/UpgradeSoulJade] 养成操作已处理, request=%v, code=%v, before=%v, after=%v, pid=%v",
		requestID, code, before, mgr.GetRevision(), uid)
}

// RefineSoulJade checks the request revision before rerolling owned modifiers.
func RefineSoulJade(ctx *core.Context, req *pb_souljade.RefineRoleSoulJadeReq) {
	uid := ctx.PlayerID()
	ack := &pb_souljade.RefineRoleSoulJadeAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	expected := req.GetExpectedRevision()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_loadout/RefineSoulJade] 收到养成操作, request=%v, expected=%v, pid=%v",
		requestID, expected, uid)

	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	ack.Data = p.BuildSoulJadeData()
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.GetCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character.GetID() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	mgr := p.GetSoulJadeMgr()
	if mgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 养成模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	before := mgr.GetRevision()
	if before == 0 {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 养成版本尚未初始化, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if requestID == 0 {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 请求编号无效, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if expected != before {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 养成版本已变化, request=%v, expected=%v, before=%v, code=%v, pid=%v",
			requestID, expected, before, ack.GetCode(), uid)
		return
	}

	code := souljade.Refine(mgr, req.GetInstanceId())
	ack.Code = code
	if code != pb_common.ErrCode_OK {
		tlog.Log.Info("[ctl_loadout/RefineSoulJade] 养成操作被拒绝, request=%v, code=%v, before=%v, after=%v, pid=%v",
			requestID, code, before, mgr.GetRevision(), uid)
		return
	}
	p.MarkDirty()
	ack.Data = p.BuildSoulJadeData()
	tlog.Log.Info("[ctl_loadout/RefineSoulJade] 养成操作已处理, request=%v, code=%v, before=%v, after=%v, pid=%v",
		requestID, code, before, mgr.GetRevision(), uid)
}

// LearnTalent validates server-owned points before learning one node.
func LearnTalent(ctx *core.Context, req *pb_talent.LearnRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.LearnRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/LearnTalent] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	expected := req.GetExpectedRevision()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_loadout/LearnTalent] 收到养成操作, request=%v, expected=%v, pid=%v",
		requestID, expected, uid)

	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	ack.Data = p.BuildTalentData()
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.GetCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character.GetID() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	mgr := p.GetTalentMgr()
	if mgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 养成模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	before := mgr.GetRevision()
	if before == 0 {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/LearnTalent] 养成版本尚未初始化, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if requestID == 0 {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/LearnTalent] 请求编号无效, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if expected != before {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/LearnTalent] 养成版本已变化, request=%v, expected=%v, before=%v, code=%v, pid=%v",
			requestID, expected, before, ack.GetCode(), uid)
		return
	}

	earned := talent.EarnedPoints(character.GetLevel(), character.GetBreakthrough())
	code := talent.Learn(mgr, req.GetNodeId(), earned)
	ack.Code = code
	if code != pb_common.ErrCode_OK {
		tlog.Log.Info("[ctl_loadout/LearnTalent] 养成操作被拒绝, request=%v, code=%v, before=%v, after=%v, pid=%v",
			requestID, code, before, mgr.GetRevision(), uid)
		return
	}
	p.MarkDirty()
	ack.Data = p.BuildTalentData()
	tlog.Log.Info("[ctl_loadout/LearnTalent] 养成操作已处理, request=%v, code=%v, before=%v, after=%v, pid=%v",
		requestID, code, before, mgr.GetRevision(), uid)
}

// ResetTalent resets the requested configured branch and refunds points.
func ResetTalent(ctx *core.Context, req *pb_talent.ResetRoleTalentReq) {
	uid := ctx.PlayerID()
	ack := &pb_talent.ResetRoleTalentAck{Code: pb_common.ErrCode_OK}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM
		tlog.Log.Info("[ctl_loadout/ResetTalent] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	expected := req.GetExpectedRevision()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_loadout/ResetTalent] 收到养成操作, request=%v, expected=%v, pid=%v",
		requestID, expected, uid)

	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	ack.Data = p.BuildTalentData()
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.GetCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character.GetID() == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	mgr := p.GetTalentMgr()
	if mgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 养成模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	before := mgr.GetRevision()
	if before == 0 {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE
		tlog.Log.Info("[ctl_loadout/ResetTalent] 养成版本尚未初始化, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if requestID == 0 {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/ResetTalent] 请求编号无效, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if expected != before {
		ack.Code = pb_common.ErrCode_STALE_REVISION
		tlog.Log.Info("[ctl_loadout/ResetTalent] 养成版本已变化, request=%v, expected=%v, before=%v, code=%v, pid=%v",
			requestID, expected, before, ack.GetCode(), uid)
		return
	}

	code := talent.Reset(mgr, req.GetBranch())
	ack.Code = code
	if code != pb_common.ErrCode_OK {
		tlog.Log.Info("[ctl_loadout/ResetTalent] 养成操作被拒绝, request=%v, code=%v, before=%v, after=%v, pid=%v",
			requestID, code, before, mgr.GetRevision(), uid)
		return
	}
	p.MarkDirty()
	ack.Data = p.BuildTalentData()
	tlog.Log.Info("[ctl_loadout/ResetTalent] 养成操作已处理, request=%v, code=%v, before=%v, after=%v, pid=%v",
		requestID, code, before, mgr.GetRevision(), uid)
}

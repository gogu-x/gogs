package ctl_battle

import (
	battleengine "github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/tlog"
)

// OnGetRoleCombatStats returns final attributes using the actual battle compiler.
func OnGetRoleCombatStats(ctx *core.Context, req *pb_common.GetRoleCombatStatsReq) {
	uid := ctx.PlayerID()
	ack := &pb_common.GetRoleCombatStatsAck{Code: pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()}
	defer ctx.CastPlayerMsg(uid, ack)

	if req == nil {
		ack.Code = pb_common.ErrCode_PARAM.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 请求为空, code=%v, pid=%v",
			ack.GetCode(), uid)
		return
	}
	requestID := req.GetRequestId()
	ack.RequestId = requestID
	tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 查询权威战斗属性, request=%v, pid=%v", requestID, uid)
	p := ctx.GetPlayer()
	if p == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 玩家数据不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 角色模块不可用, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	character, hasCharacter := roleMgr.BattleCharacter()
	if !hasCharacter {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 角色尚未加载, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	if character == nil {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 角色数据为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	roleID := character.GetID()
	if roleID == "" {
		ack.Code = pb_common.ErrCode_LOADOUT_UNAVAILABLE.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 角色编号为空, request=%v, code=%v, pid=%v",
			requestID, ack.GetCode(), uid)
		return
	}
	snapshot, code := battleRoleMessage(p, character)
	if code != pb_common.ErrCode_OK {
		ack.Code = code.Enum()
		tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 战斗快照不可用, request=%v, code=%v, pid=%v",
			requestID, code, uid)
		return
	}
	attributes, err := battleengine.NewDefaultConfigCompiler().CalculateRoleAttributes(snapshot)
	if err != nil {
		ack.Code = pb_common.ErrCode_CONFIG_UNAVAILABLE.Enum()
		tlog.Log.Error("[ctl_battle/OnGetRoleCombatStats] 编译战斗属性失败, request=%v, code=%v, pid=%v, err=%v",
			requestID, ack.GetCode(), uid, err)
		return
	}
	ack.Data = &pb_common.RoleCombatStatsData{
		RoleId: roleID, MaxHp: attributes.GetMaxHP(), Attack: attributes.GetAttack(),
		Defense: attributes.GetDefense(), Speed: attributes.GetSpeed(),
	}
	ack.Code = pb_common.ErrCode_OK.Enum()
	tlog.Log.Info("[ctl_battle/OnGetRoleCombatStats] 返回权威战斗属性, request=%v, role=%v, hp=%v, attack=%v, defense=%v, speed=%v, pid=%v",
		requestID, roleID, attributes.GetMaxHP(), attributes.GetAttack(), attributes.GetDefense(), attributes.GetSpeed(), uid)
}

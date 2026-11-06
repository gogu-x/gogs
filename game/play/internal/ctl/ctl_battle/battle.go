package ctl_battle

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// OnCreateBattle submits an authoritative request assembled from loaded player state.
func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	uid := ctx.PlayerID()
	request, code := buildBattleRequest(ctx.GetPlayer(), req)
	if code != pb_common.ErrCode_OK {
		tlog.Log.Error("[ctl_battle/OnCreateBattle] 组装战斗请求失败, pid=%v code=%v", uid, code)
		ctx.CastPlayerMsg(uid, &pb_battle.StartBattleAck{Code: code})
		return
	}

	ok := ctx.CastCall(def.BattleClient, &battle.Begin{Request: request}, func(callbackCtx tree.Context, value interface{}, err error) {
		ack := battleStartAck(value, err)
		if ack.GetCode() != pb_common.ErrCode_OK {
			tlog.Log.Error("[ctl_battle/OnCreateBattle] 创建战斗失败, pid=%v code=%v err=%v response=%T", uid, ack.GetCode(), err, value)
		}
		pushBattleAck(callbackCtx, uid, ack)
	})
	if !ok {
		ctx.CastPlayerMsg(uid, &pb_battle.StartBattleAck{Code: pb_common.ErrCode_BATTLE_UNAVAILABLE})
	}
}

func battleStartAck(value interface{}, err error) *pb_battle.StartBattleAck {
	ack := &pb_battle.StartBattleAck{Code: pb_common.ErrCode_OK}
	state, valid := value.(battle.State)
	if valid {
		ack.BattleId = state.BattleID
	}
	if errors.Is(err, context.DeadlineExceeded) {
		ack.Code = pb_common.ErrCode_BATTLE_TIMEOUT
		return ack
	}
	if errors.Is(err, tree.ErrTimeout) {
		ack.Code = pb_common.ErrCode_BATTLE_TIMEOUT
		return ack
	}
	if errors.Is(err, battle.ErrUnavailable) {
		ack.Code = pb_common.ErrCode_BATTLE_UNAVAILABLE
		return ack
	}
	if errors.Is(err, battle.ErrInvalidRequest) {
		ack.Code = pb_common.ErrCode_INVALID_BATTLE_REQUEST
		return ack
	}
	if errors.Is(err, battle.ErrInProgress) {
		ack.Code = pb_common.ErrCode_BATTLE_IN_PROGRESS
		return ack
	}
	if err != nil {
		ack.Code = pb_common.ErrCode_BATTLE_CREATE_FAILED
		return ack
	}
	if !valid {
		ack.Code = pb_common.ErrCode_BATTLE_CREATE_FAILED
		return ack
	}
	return ack
}

// buildBattleRequest reads the current character, loadout, and tower state.
func buildBattleRequest(currentPlayer *player.Player, incoming *pb_battle.StartBattleReq) (*pb_battle.StartBattleReq, pb_common.ErrCode) {
	if currentPlayer == nil {
		return nil, pb_common.ErrCode_INVALID_BATTLE_REQUEST
	}
	uid := currentPlayer.GetUID()
	if uid == 0 {
		return nil, pb_common.ErrCode_INVALID_BATTLE_REQUEST
	}
	towerMgr := currentPlayer.GetTowerMgr()
	if towerMgr == nil {
		return nil, pb_common.ErrCode_BATTLE_UNAVAILABLE
	}
	if incoming == nil {
		return nil, pb_common.ErrCode_INVALID_BATTLE_REQUEST
	}
	roleMgr := currentPlayer.GetRoleMgr()
	if roleMgr == nil {
		return nil, pb_common.ErrCode_BATTLE_UNAVAILABLE
	}
	character, ok := roleMgr.BattleCharacter()
	if !ok {
		return nil, pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	characterMessage, code := battleRoleMessage(currentPlayer, character)
	if code != pb_common.ErrCode_OK {
		return nil, code
	}
	towerLayer := towerMgr.GetLayer()

	return &pb_battle.StartBattleReq{
		UID:                  uid,
		ServerID:             uint32(conf.ServerID),
		SourceNodeId:         uint32(conf.NodeId),
		BattleType:           pb_battle.BattleType_BATTLE_TYPE_TOWER,
		BusinessId:           fmt.Sprintf("tower-%d-%d", uid, towerLayer),
		AttackerRoles:        []*pb_battle.Role{characterMessage},
		MonsterGroupConfigId: towerLayer,
	}, pb_common.ErrCode_OK
}

func battleRoleMessage(currentPlayer *player.Player, selectedRole *role.Role) (*pb_battle.Role, pb_common.ErrCode) {
	if currentPlayer == nil {
		return nil, pb_common.ErrCode_INVALID_BATTLE_REQUEST
	}
	if selectedRole == nil {
		return nil, pb_common.ErrCode_INVALID_BATTLE_REQUEST
	}
	roleID := selectedRole.GetID()
	roleConfigID := selectedRole.GetConfigID()
	skills := selectedRole.GetSkills()
	equipmentIDs := selectedRole.GetEquipmentIDs()
	equipmentMgr := currentPlayer.GetEquipmentMgr()
	if equipmentMgr == nil {
		return nil, pb_common.ErrCode_EQUIPMENT_UNAVAILABLE
	}
	roleSkills := make([]*pb_battle.RoleSkill, 0, len(skills))
	for _, skill := range skills {
		roleSkills = append(roleSkills, &pb_battle.RoleSkill{SkillConfigId: skill.GetConfigID(), Level: skill.GetLevel()})
	}
	roleEquips := make([]*pb_battle.RoleEquip, 0, len(equipmentIDs))
	for _, equipmentID := range equipmentIDs {
		equip, ok := equipmentMgr.Get(equipmentID)
		if !ok {
			tlog.Log.Error("[ctl_battle/battleRoleMessage] 角色装备不存在, pid=%v role=%v equipment=%v", currentPlayer.GetUID(), roleID, equipmentID)
			return nil, pb_common.ErrCode_EQUIPMENT_UNAVAILABLE
		}
		roleEquips = append(roleEquips, &pb_battle.RoleEquip{
			EquipId: equip.GetID(), EquipConfigId: equip.GetConfigID(),
			Level: equip.GetLevel(), RefineLevel: equip.GetRefineLevel(),
		})
	}
	message := &pb_battle.Role{
		RoleId:                 roleID,
		RoleConfigId:           roleConfigID,
		Level:                  selectedRole.GetLevel(),
		Star:                   selectedRole.GetStar(),
		Breakthrough:           selectedRole.GetBreakthrough(),
		Skills:                 roleSkills,
		Equips:                 roleEquips,
		SoulJadeEffectAffixIds: souljade.EffectAffixIDs(currentPlayer.GetSoulJadeMgr()),
		GrowthBonus:            currentPlayer.BattleBonuses(),
	}
	return message, pb_common.ErrCode_OK
}

// OnBattleFinished 在当前玩家角色中处理结算并触发玩家级业务事件。
func OnBattleFinished(ctx *core.Context, settlement *battle.Settlement) {
	if ctx == nil || settlement == nil || settlement.BattleID == "" {
		return
	}
	uid := ctx.PlayerID()
	participantFound := false
	for _, participant := range settlement.Participants {
		if participant.UID == uid {
			participantFound = true
			tlog.Log.Info("[ctl_battle/OnBattleFinished] 收到玩家战斗结算, pid=%v battleID=%v server=%v node=%v type=%v outcome=%v units=%v checksum=%v",
				uid, settlement.BattleID, participant.ServerID, participant.NodeID,
				settlement.BattleType, settlement.Outcome,
				len(settlement.Units), settlement.Checksum)
			break
		}
	}
	if !participantFound {
		tlog.Log.Error("[ctl_battle/OnBattleFinished] 结算中没有当前玩家, pid=%v battleID=%v", uid, settlement.BattleID)
		return
	}

	arg := comm.NewArg()
	arg.Set("settlement", settlement)
	ctx.Emit(core.BattleSettled, arg)
}

func pushBattleAck(ctx tree.Context, uid uint64, ack *pb_battle.StartBattleAck) {
	if gate, ok := ctx.Lookup(def.GameGate); ok {
		if !ctx.Send(gate, &ipb.PushToMsg{UID: uid, Msg: ack}) {
			tlog.Log.Error("[ctl_battle/pushBattleAck] 战斗响应投递失败, pid=%v", uid)
		}
		return
	}
	tlog.Log.Error("[ctl_battle/pushBattleAck] 网关 Actor 不可用, pid=%v", uid)
}

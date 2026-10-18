package ctl_battle

import (
	"fmt"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// OnCreateBattle 从玩家内存数据组装权威战斗请求并发送给战斗服务。
func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	uid := ctx.PlayerID()
	request, err := buildBattleRequest(ctx.GetPlayer(), req)
	if err != nil {
		tlog.Log.Error("[ctl_battle/OnCreateBattle] 组装战斗请求失败, pid=%v err=%v", uid, err)
		ctx.CastPlayerMsg(uid, &pb_battle.StartBattleAck{Error: err.Error()})
		return
	}

	ok := ctx.CastCall(def.BattleClient, &battle.Begin{Request: request}, func(callbackCtx tree.Context, value interface{}, err error) {
		ack := &pb_battle.StartBattleAck{}
		if state, valid := value.(battle.State); valid {
			ack.BattleId = state.BattleID
		}
		if err != nil {
			ack.Error = err.Error()
		}
		pushBattleAck(callbackCtx, uid, ack)
	})
	if !ok {
		ctx.CastPlayerMsg(uid, &pb_battle.StartBattleAck{Error: "battle client is unavailable"})
	}
}

// buildBattleRequest 从玩家当前角色、装备和爬塔数据组装战斗请求。
func buildBattleRequest(currentPlayer *player.Player, incoming *pb_battle.StartBattleReq) (*pb_battle.StartBattleReq, error) {
	if currentPlayer == nil {
		return nil, fmt.Errorf("battle request requires a valid player")
	}
	uid := currentPlayer.GetUID()
	if uid == 0 {
		return nil, fmt.Errorf("battle request requires a valid player")
	}
	towerMgr := currentPlayer.GetTowerMgr()
	if towerMgr == nil {
		return nil, fmt.Errorf("player %d has no tower state", uid)
	}
	if incoming == nil {
		return nil, fmt.Errorf("battle request is required")
	}
	roleMgr := currentPlayer.GetRoleMgr()
	if roleMgr == nil {
		return nil, fmt.Errorf("player %d has no role state", uid)
	}
	character, ok := roleMgr.BattleCharacter()
	if !ok {
		return nil, fmt.Errorf("player %d has no configured character", uid)
	}
	characterMessage, err := battleRoleMessage(currentPlayer, character)
	if err != nil {
		return nil, err
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
	}, nil
}

func battleRoleMessage(currentPlayer *player.Player, selectedRole *role.Role) (*pb_battle.Role, error) {
	if currentPlayer == nil || selectedRole == nil {
		return nil, fmt.Errorf("battle role and player are required")
	}
	roleID := selectedRole.GetID()
	roleConfigID := selectedRole.GetConfigID()
	skills := selectedRole.GetSkills()
	equipmentIDs := selectedRole.GetEquipmentIDs()
	equipmentMgr := currentPlayer.GetEquipmentMgr()
	if equipmentMgr == nil {
		return nil, fmt.Errorf("player %d has no equipment state", currentPlayer.GetUID())
	}
	roleSkills := make([]*pb_battle.RoleSkill, 0, len(skills))
	for _, skill := range skills {
		roleSkills = append(roleSkills, &pb_battle.RoleSkill{SkillConfigId: skill.GetConfigID(), Level: skill.GetLevel()})
	}
	roleEquips := make([]*pb_battle.RoleEquip, 0, len(equipmentIDs))
	for _, equipmentID := range equipmentIDs {
		equip, ok := equipmentMgr.Get(equipmentID)
		if !ok {
			return nil, fmt.Errorf("role %q references missing equipment %q", roleID, equipmentID)
		}
		roleEquips = append(roleEquips, &pb_battle.RoleEquip{
			EquipId: equip.GetID(), EquipConfigId: equip.GetConfigID(),
			Level: equip.GetLevel(), RefineLevel: equip.GetRefineLevel(),
		})
	}
	message := &pb_battle.Role{
		RoleId:       roleID,
		RoleConfigId: roleConfigID,
		Level:        selectedRole.GetLevel(),
		Star:         selectedRole.GetStar(),
		Breakthrough: selectedRole.GetBreakthrough(),
		Skills:       roleSkills,
		Equips:       roleEquips,
	}
	return message, nil
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

package ctl_battle

import (
	"fmt"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/game/play/internal/module/player/role"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// OnCreateBattle 从玩家各个数据模块读取权威数据，组装战斗请求并发送给战斗服务。
func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	uid := ctx.PlayerID()
	request, err := buildBattleRequest(ctx.Player, req)
	if err != nil {
		ctx.CastPlayerIdMsg(uid, &pb_battle.StartBattleAck{Error: err.Error()})
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
		pushAck(callbackCtx, uid, ack)
	})
	if !ok {
		ctx.CastPlayerIdMsg(uid, &pb_battle.StartBattleAck{Error: "battle client is unavailable"})
	}
}

func buildBattleRequest(player *player.Player, incoming *pb_battle.StartBattleReq) (*pb_battle.StartBattleReq, error) {
	if player == nil || player.UID == 0 {
		return nil, fmt.Errorf("battle request requires a valid player")
	}
	if player.TowerMgr == nil {
		return nil, fmt.Errorf("player %d has no tower state", player.UID)
	}
	if incoming == nil {
		return nil, fmt.Errorf("battle request is required")
	}
	character, ok := player.RoleMgr.BattleCharacter()
	if !ok {
		return nil, fmt.Errorf("player %d has no configured character", player.UID)
	}
	characterMessage, err := battleRoleMessage(player, character)
	if err != nil {
		return nil, err
	}

	return &pb_battle.StartBattleReq{
		UID:                  player.UID,
		ServerID:             uint32(conf.ServerID),
		SourceNodeId:         uint32(conf.NodeId),
		BattleType:           pb_battle.BattleType_BATTLE_TYPE_TOWER,
		BusinessId:           fmt.Sprintf("tower-%d-%d", player.UID, player.TowerMgr.Layer),
		AttackerRoles:        []*pb_battle.Role{characterMessage},
		MonsterGroupConfigId: player.TowerMgr.Layer,
	}, nil
}

func battleRoleMessage(player *player.Player, selectedRole *role.Role) (*pb_battle.Role, error) {
	message := &pb_battle.Role{
		RoleId:       selectedRole.ID,
		RoleConfigId: selectedRole.ConfigID,
		Level:        selectedRole.Level,
		Star:         selectedRole.Star,
		Breakthrough: selectedRole.Breakthrough,
		Skills:       make([]*pb_battle.RoleSkill, 0, len(selectedRole.Skills)),
		Equips:       make([]*pb_battle.RoleEquip, 0, len(selectedRole.EquipmentIDs)),
	}
	for _, skill := range selectedRole.Skills {
		message.Skills = append(message.Skills, &pb_battle.RoleSkill{SkillConfigId: skill.ConfigID, Level: skill.Level})
	}
	for _, equipmentID := range selectedRole.EquipmentIDs {
		equip, ok := player.EquipmentMgr.Get(equipmentID)
		if !ok {
			return nil, fmt.Errorf("role %q references missing equipment %q", selectedRole.ID, equipmentID)
		}
		message.Equips = append(message.Equips, &pb_battle.RoleEquip{
			EquipId: equip.ID, EquipConfigId: equip.ConfigID,
			Level: equip.Level, RefineLevel: equip.RefineLevel,
		})
	}
	return message, nil
}

func pushAck(ctx tree.Context, uid uint64, ack *pb_battle.StartBattleAck) {
	if gate, ok := ctx.Lookup(def.GameGate); ok {
		ctx.Send(gate, &ipb.PushToMsg{UID: uid, Msg: ack})
	}
}

// OnBattleFinished 结算一场战斗。
//
// 目前只打日志 + 广播 BattleSettled —— 玩家数据层（PlayerMgr.Loader/Save）还是空实现，
// 没有地方落库。等数据层接上后在这里按 Settlement 改数据即可：投递方（game/battle）
// 和订阅方（任务/成就/活动）都不需要动。
//
// 幂等提醒：同一场战斗的结果可能重复投递（NATS 重发、Game 重连后的补投）。现在只写
// 日志无所谓，真接数据层时必须以 BattleID 做去重键。
func OnBattleFinished(ctx *core.Context, settlement *battle.Settlement) {
	if settlement == nil || settlement.BattleID == "" {
		return
	}

	for _, p := range settlement.Participants {
		// 刻意不去 PlayerMgr 查在线玩家：战斗现在要跑十几秒，玩家很可能中途掉线，
		// 查在线会让整个结算被静默跳过。接数据层时应当按 uid 走 load → 改 → save，
		// 把结果推给客户端只是附带的一步，离线就跳过。
		tlog.Log.Info("play/battle-settle: battleID=%v uid=%v server=%v node=%v type=%v outcome=%v units=%v checksum=%v",
			settlement.BattleID, p.UID, p.ServerID, p.NodeID,
			settlement.BattleType, settlement.Outcome,
			len(settlement.Units), settlement.Checksum)
	}

	// 广播给任务/成就/活动等模块。Arg 里放结算输入本身，订阅方按需自取。
	arg := comm.NewArg()
	arg.Set("settlement", settlement)
	ctx.Emit(core.BattleSettled, arg)
}

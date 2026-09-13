package ctl_battle

import (
	"fmt"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// OnCreateBattle 把客户端的“开始战斗”意图转换为服务端权威战斗快照。
// 当前玩家数据尚未持有角色/装备/技能养成信息，因此最小演示固定使用配置 1；
// 后续只需替换 buildDemoRequest，BattleClient 和 Unity 事件表现无需改动。
func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	uid := ctx.PlayerID()
	request := buildDemoRequest(ctx.Player, req)

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

func buildDemoRequest(player *player.Player, incoming *pb_battle.StartBattleReq) *pb_battle.StartBattleReq {
	request := &pb_battle.StartBattleReq{}
	if incoming != nil {
		request = proto.Clone(incoming).(*pb_battle.StartBattleReq)
	}
	request.UID = player.UID
	request.ServerID = uint32(conf.ServerID)
	request.SourceNodeId = uint32(conf.NodeId)
	request.BattleType = pb_battle.BattleType_BATTLE_TYPE_TOWER
	request.BusinessId = fmt.Sprintf("client-demo-%d", player.UID)

	request.AttackerRoles = []*pb_battle.Role{{
		RoleId:       fmt.Sprintf("player-%d", player.UID),
		RoleConfigId: 1,
		Level:        1,
	}}
	request.DefenderRoles = nil
	request.MonsterGroupConfigId = player.TowerMgr.Layer
	return request
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

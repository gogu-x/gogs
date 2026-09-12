package ctl_battle

import (
	"fmt"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battleclient"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

const (
	demoRoleConfigID   int32 = 1
	demoMonsterGroupID int32 = 1
)

// OnCreateBattle 把客户端的“开始战斗”意图转换为服务端权威战斗快照。
// 当前玩家数据尚未持有角色/装备/技能养成信息，因此最小演示固定使用配置 1；
// 后续只需替换 buildDemoRequest，BattleClient 和 Unity 事件表现无需改动。
func OnCreateBattle(ctx *core.Context, req *pb_battle.StartBattleReq) {
	uid := ctx.PlayerID()
	request := buildDemoRequest(uid, req)

	ok := ctx.CastCall(def.BattleClient, &battleclient.Begin{Request: request}, func(callbackCtx tree.Context, value interface{}, err error) {
		ack := &pb_battle.StartBattleAck{}
		if state, valid := value.(battleclient.State); valid {
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

func buildDemoRequest(uid uint64, incoming *pb_battle.StartBattleReq) *pb_battle.StartBattleReq {
	request := &pb_battle.StartBattleReq{}
	if incoming != nil {
		request = proto.Clone(incoming).(*pb_battle.StartBattleReq)
	}
	request.UID = uid
	request.ServerID = uint32(conf.ServerID)
	request.SourceNodeId = uint32(conf.NodeId)
	request.BattleType = pb_battle.BattleType_BATTLE_TYPE_TEST
	request.BusinessId = fmt.Sprintf("client-demo-%d", uid)
	request.AttackerRoles = []*pb_battle.Role{{
		RoleId:       fmt.Sprintf("player-%d", uid),
		RoleConfigId: demoRoleConfigID,
		Level:        1,
	}}
	request.DefenderRoles = nil
	request.MonsterGroupConfigId = demoMonsterGroupID
	return request
}

func pushAck(ctx tree.Context, uid uint64, ack *pb_battle.StartBattleAck) {
	if gate, ok := ctx.Lookup(def.GameGate); ok {
		ctx.Send(gate, &ipb.PushToMsg{UID: uid, Msg: ack})
	}
}

package ctl_battle

import (
	"testing"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree/tlog"
)

// testCtx 造一个只用于触发广播的最小 Context。
// Play.Event 为 nil 是安全的：comm.Event.Emit 最终走 Dispatch，而 Dispatch
// 对 nil 接收者直接返回 0。这里不涉及任何请求回复，所以 TreeCtx 留空。
func testCtx() *core.Context {
	return &core.Context{Play: &core.Play{}}
}

// TestBuildDemoRequestOverridesClientSnapshot 锁定服务端权威：客户端传来的
// UID / 服务器信息 / 参战阵容 / 怪物组一律被覆盖，且不改动入参。
func TestBuildDemoRequestOverridesClientSnapshot(t *testing.T) {
	oldServerID, oldNodeID := conf.ServerID, conf.NodeId
	conf.ServerID, conf.NodeId = 3, 4
	t.Cleanup(func() { conf.ServerID, conf.NodeId = oldServerID, oldNodeID })

	p := player.NewPlayerData(99)
	incoming := &pb.StartBattleReq{
		UID:                  123,
		ServerID:             456,
		SourceNodeId:         789,
		BattleType:           pb.BattleType_BATTLE_TYPE_PVP,
		AttackerRoles:        []*pb.Role{{RoleId: "forged", RoleConfigId: 999}},
		DefenderRoles:        []*pb.Role{{RoleId: "forged-defender", RoleConfigId: 999}},
		MonsterGroupConfigId: 999,
	}

	request := buildDemoRequest(p, incoming)

	if request.GetUID() != 99 || request.GetServerID() != 3 || request.GetSourceNodeId() != 4 {
		t.Fatalf("source header = uid:%d server:%d node:%d",
			request.GetUID(), request.GetServerID(), request.GetSourceNodeId())
	}
	if request.GetBattleType() != pb.BattleType_BATTLE_TYPE_TOWER {
		t.Fatalf("battle type = %s, want TOWER", request.GetBattleType())
	}
	// 怪物组取自玩家的爬塔层数，不是客户端传来的 999。
	if request.GetMonsterGroupConfigId() != p.TowerMgr.Layer {
		t.Fatalf("monster group = %d, want %d",
			request.GetMonsterGroupConfigId(), p.TowerMgr.Layer)
	}
	if len(request.GetAttackerRoles()) != 1 ||
		request.GetAttackerRoles()[0].GetRoleId() != "player-99" ||
		request.GetAttackerRoles()[0].GetRoleConfigId() != 1 {
		t.Fatalf("attacker roles = %#v", request.GetAttackerRoles())
	}
	if len(request.GetDefenderRoles()) != 0 {
		t.Fatalf("defender roles = %#v", request.GetDefenderRoles())
	}
	if request.GetBusinessId() != "client-demo-99" {
		t.Fatalf("business id = %q", request.GetBusinessId())
	}

	// buildDemoRequest 必须克隆入参，不能就地改写调用方的请求。
	if incoming.GetUID() != 123 || incoming.GetBattleType() != pb.BattleType_BATTLE_TYPE_PVP ||
		len(incoming.GetAttackerRoles()) != 1 || incoming.GetAttackerRoles()[0].GetRoleId() != "forged" {
		t.Fatalf("incoming request was mutated: %#v", incoming)
	}
}

// TestOnBattleFinishedToleratesEmptyInput 冒烟：结算入口不能因为输入不完整而 panic。
// 真正的落库断言要等玩家数据层接上之后再补。
func TestOnBattleFinishedToleratesEmptyInput(t *testing.T) {
	tlog.NewLog(conf.LogPath, 0)

	OnBattleFinished(testCtx(), nil)
	OnBattleFinished(testCtx(), &battle.Settlement{}) // 缺 BattleID
}

// TestOnBattleFinishedAcceptsOfflineParticipant 锁定一个刻意的设计取舍：
// 结算不检查玩家是否在线。战斗要跑十几秒，玩家掉线很常见，查在线会让结算被静默跳过。
func TestOnBattleFinishedAcceptsOfflineParticipant(t *testing.T) {
	tlog.NewLog(conf.LogPath, 0)

	// 注意这里没有往任何 PlayerMgr 里注册玩家，模拟"结果到达时玩家已离线"。
	OnBattleFinished(testCtx(), &battle.Settlement{
		BattleID:   "battle-offline-1",
		BattleType: pb.BattleType_BATTLE_TYPE_TOWER,
		Outcome:    pb.BattleOutcome_BATTLE_OUTCOME_ATTACKER_WIN,
		Participants: []battle.Participant{
			{UID: 1001, ServerID: 1, NodeID: 1},
		},
	})
}

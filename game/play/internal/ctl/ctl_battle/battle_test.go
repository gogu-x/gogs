package ctl_battle

import (
	"testing"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/game/play/internal/module/player/equipment"
	"github.com/gogu-x/gogs/game/play/internal/module/player/role"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree/tlog"
)

// testCtx 造一个只用于触发广播的最小 Context。
// Play.Event 为 nil 是安全的：comm.Event.Emit 最终走 Dispatch，而 Dispatch
// 对 nil 接收者直接返回 0。这里不涉及任何请求回复，所以 TreeCtx 留空。
func testCtx() *core.Context {
	return &core.Context{Play: &core.Play{}}
}

// TestBuildBattleRequestCopiesLoadout 检查请求不携带参战角色时，Game 仍会从玩家模块
// 读取唯一角色、角色养成和装备数据，并覆盖请求中的玩家身份信息。
func TestBuildBattleRequestCopiesLoadout(t *testing.T) {
	oldServerID, oldNodeID := conf.ServerID, conf.NodeId
	conf.ServerID, conf.NodeId = 3, 4
	t.Cleanup(func() { conf.ServerID, conf.NodeId = oldServerID, oldNodeID })

	p := player.NewPlayerData(99)
	p.RoleMgr.Character = &role.Role{
		ID: "role-99", ConfigID: 1, Level: 8, Star: 2, Breakthrough: 1,
		Skills: []role.Skill{{ConfigID: 11, Level: 3}}, EquipmentIDs: []string{"equip-1"},
	}
	p.EquipmentMgr.Items["equip-1"] = &equipment.Item{ID: "equip-1", ConfigID: 7, Level: 3, RefineLevel: 2}
	incoming := &pb.StartBattleReq{
		UID:                  123,
		ServerID:             456,
		SourceNodeId:         789,
		BattleType:           pb.BattleType_BATTLE_TYPE_PVP,
		MonsterGroupConfigId: 999,
	}

	request, err := buildBattleRequest(p, incoming)
	if err != nil {
		t.Fatal(err)
	}

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
		request.GetAttackerRoles()[0].GetRoleId() != "role-99" ||
		request.GetAttackerRoles()[0].GetRoleConfigId() != 1 ||
		request.GetAttackerRoles()[0].GetLevel() != 8 ||
		request.GetAttackerRoles()[0].GetStar() != 2 ||
		request.GetAttackerRoles()[0].GetBreakthrough() != 1 ||
		len(request.GetAttackerRoles()[0].GetSkills()) != 1 ||
		request.GetAttackerRoles()[0].GetSkills()[0].GetLevel() != 3 ||
		len(request.GetAttackerRoles()[0].GetEquips()) != 1 ||
		request.GetAttackerRoles()[0].GetEquips()[0].GetEquipConfigId() != 7 ||
		request.GetAttackerRoles()[0].GetEquips()[0].GetLevel() != 3 ||
		request.GetAttackerRoles()[0].GetEquips()[0].GetRefineLevel() != 2 {
		t.Fatalf("attacker roles = %#v", request.GetAttackerRoles())
	}
	if len(request.GetDefenderRoles()) != 0 {
		t.Fatalf("defender roles = %#v", request.GetDefenderRoles())
	}
	if request.GetBusinessId() != "tower-99-1" {
		t.Fatalf("business id = %q", request.GetBusinessId())
	}

	// buildBattleRequest 必须克隆入参，不能就地改写调用方的请求。
	if incoming.GetUID() != 123 || incoming.GetBattleType() != pb.BattleType_BATTLE_TYPE_PVP ||
		len(incoming.GetAttackerRoles()) != 0 || incoming.GetMonsterGroupConfigId() != 999 {
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

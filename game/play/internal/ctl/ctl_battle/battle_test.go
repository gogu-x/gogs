package ctl_battle

import (
	"testing"

	"github.com/gogu-x/gogs/conf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

func TestBuildDemoRequestOverridesClientSnapshot(t *testing.T) {
	oldServerID, oldNodeID := conf.ServerID, conf.NodeId
	conf.ServerID, conf.NodeId = 3, 4
	t.Cleanup(func() { conf.ServerID, conf.NodeId = oldServerID, oldNodeID })

	request := buildDemoRequest(99, &pb.StartBattleReq{
		UID:                  123,
		ServerID:             456,
		SourceNodeId:         789,
		BattleType:           pb.BattleType_BATTLE_TYPE_PVP,
		AttackerRoles:        []*pb.Role{{RoleId: "forged", RoleConfigId: 999}},
		DefenderRoles:        []*pb.Role{{RoleId: "forged-defender", RoleConfigId: 999}},
		MonsterGroupConfigId: 999,
	})

	if request.GetUID() != 99 || request.GetServerID() != 3 || request.GetSourceNodeId() != 4 {
		t.Fatalf("source header = uid:%d server:%d node:%d", request.GetUID(), request.GetServerID(), request.GetSourceNodeId())
	}
	if request.GetBattleType() != pb.BattleType_BATTLE_TYPE_TEST || request.GetMonsterGroupConfigId() != demoMonsterGroupID {
		t.Fatalf("battle setup = type:%s group:%d", request.GetBattleType(), request.GetMonsterGroupConfigId())
	}
	if len(request.GetAttackerRoles()) != 1 || request.GetAttackerRoles()[0].GetRoleId() != "player-99" || request.GetAttackerRoles()[0].GetRoleConfigId() != demoRoleConfigID {
		t.Fatalf("attacker roles = %#v", request.GetAttackerRoles())
	}
	if len(request.GetDefenderRoles()) != 0 {
		t.Fatalf("defender roles = %#v", request.GetDefenderRoles())
	}
}

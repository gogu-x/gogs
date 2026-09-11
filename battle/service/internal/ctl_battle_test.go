package internal

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

func installManagerConfigs(t *testing.T) {
	t.Helper()
	err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{{CfgID: 1, MaxHP: 100, Attack: 10, Defense: 1, Speed: 10, DefaultPosition: 1, BasicSkillID: 1}},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), Effects: []glconf.BattleEffectCfg{{Kind: 1, CoefficientPermille: 1000}}}},
		nil,
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 10, Attack: 1, Defense: 0, Speed: 1, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []glconf.BattleMonsterGroupMemberCfg{{MonsterCfgID: 1, Position: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_PVE), ATBThreshold: 1000, MaxActions: 5, CritMultiplierPermille: 1500}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateBattleSpawnsAndTracksActor(t *testing.T) {
	installManagerConfigs(t)
	server := New()
	server.notifier = battle.NotifyFunc(func(_ battle.Source, _ proto.Message) error { return nil })
	treeSystem := tree.NewTree()
	managerPID := treeSystem.SpawnOne(server)
	t.Cleanup(treeSystem.Shutdown)
	request := &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_PVE, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
	value, err := treeSystem.Request(managerPID, request).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ack := value.(*pb.StartBattleAck)
	if ack.GetBattleId() == "" {
		t.Fatal("missing generated battle id")
	}
	actorPID, exists := server.active[ack.GetBattleId()]
	if !exists || actorPID.ID == 0 {
		t.Fatalf("active = %#v", server.active)
	}
	treeSystem.Send(managerPID, &battle.ActorStopped{BattleID: ack.GetBattleId(), PID: actorPID})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, exists := server.active[ack.GetBattleId()]; !exists {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("active battle was not removed: %#v", server.active)
}

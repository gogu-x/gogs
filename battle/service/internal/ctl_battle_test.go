package internal

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	cspb "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

func MainTest(t *testing.T) {
	t.Helper()
	tlog.NewLog(conf.LogPath, 0)
	err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{{CfgID: 1, MaxHP: 100, Attack: 20, Defense: 5, Speed: 10, DefaultPosition: 1, BasicSkillID: 1}},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), Effects: []*cspb.TypIDVal{{Typ: "battle_damage", Pro: 1000}}}},
		nil,
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 40, Attack: 10, Defense: 2, Speed: 8, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*cspb.TypIDVal{{Typ: "battle_monster", Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_PVE), ATBThreshold: 100, MaxActions: 20, TickDurationMS: 100, CritMultiplierPermille: 1500}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateBattleSpawnsAndTracksActor(t *testing.T) {
	MainTest(t)
	server := New()
	finished := make(chan struct{}, 1)
	var createdTickDuration int32
	server.notifier = battle.NotifyFunc(func(_ battle.Source, message proto.Message) error {
		switch value := message.(type) {
		case *pb.BattleCreatedNtf:
			createdTickDuration = value.GetTickDurationMs()
		case *pb.BattleFinishedNtf:
			select {
			case finished <- struct{}{}:
			default:
			}
		}
		return nil
	})
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
	select {
	case <-finished:
		if createdTickDuration != 100 {
			t.Fatalf("created tick duration = %d, want 100", createdTickDuration)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("battle did not finish")
	}
	treeSystem.Send(managerPID, &battle.ActorStopped{BattleID: ack.GetBattleId(), PID: actorPID})
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, exists := server.active[ack.GetBattleId()]; !exists {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if _, exists := server.active[ack.GetBattleId()]; exists {
		t.Fatalf("active battle was not removed: %#v", server.active)
	}
}

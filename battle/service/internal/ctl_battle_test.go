package internal

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

func MainTest() {
	tlog.NewLog(conf.LogPath, 0)
	conf.GconfDbUri = "mongodb://admin:asd620522-@43.160.212.55:27018/?authSource=admin"
	conf.GconfDb = "gs_conf_g1_dev"
	//加载配置表
	err := glconf.LoadAllConfs(conf.GconfDbUri, conf.GconfDb, true)
	if err != nil {
		tlog.Log.Error("load confs error: %v", err)
	}
}

func TestCreateBattleSpawnsAndTracksActor(t *testing.T) {
	MainTest()
	server := New()
	finished := make(chan struct{}, 1)
	server.notifier = battle.NotifyFunc(func(_ battle.Source, message proto.Message) error {
		if _, ok := message.(*pb.BattleFinishedNtf); ok {
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
	request := &pb.StartBattleReq{Uid: 7, Sid: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_PVE, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
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

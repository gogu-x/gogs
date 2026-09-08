package main_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/service"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/battleclient"
	"github.com/gogu-x/gogs/ipb"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

type localBridge struct{ tr *tree.Tree }

func (b *localBridge) Notify(source service.Source, msg proto.Message) error {
	if source.ServerID != 10 || source.NodeID != 11 {
		return fmt.Errorf("unexpected game source %+v", source)
	}
	pid, ok := b.tr.Lookup(def.BattleClient)
	if !ok || !b.tr.Send(pid, msg) {
		return fmt.Errorf("battle client unavailable")
	}
	return nil
}

func (b *localBridge) Start(_ battleclient.Node, req *pb.StartBattleReq) error {
	pid, ok := b.tr.Lookup(def.BattleService)
	if !ok || !b.tr.Send(pid, req) {
		return fmt.Errorf("battle service unavailable")
	}
	return nil
}

func (b *localBridge) Confirm(_ battleclient.Node, msg *pb.BattleResultConfirmedNtf) error {
	pid, ok := b.tr.Lookup(def.BattleService)
	if !ok || !b.tr.Send(pid, msg) {
		return fmt.Errorf("battle service unavailable")
	}
	return nil
}

type oneNode struct{}

func (oneNode) Pick() (battleclient.Node, error) {
	return battleclient.Node{ServerID: 20, NodeID: 21}, nil
}

type e2eGate struct{ messages chan proto.Message }

func (g *e2eGate) Name() string        { return def.GameGate }
func (g *e2eGate) OnInit(tree.Context) {}
func (g *e2eGate) OnStop(tree.Context) {}
func (g *e2eGate) HandleMessage(_ tree.Context, msg interface{}) {
	if push, ok := msg.(*ipb.PushToMsg); ok {
		if wire, ok := push.Msg.(proto.Message); ok {
			g.messages <- wire
		}
	}
}

func TestGameBattleEndToEndWithoutExternalServices(t *testing.T) {
	configs, err := service.LoadConfigRepository("")
	if err != nil {
		t.Fatal(err)
	}
	reports := service.NewMemoryRepository()
	tr := tree.NewTree()
	bridge := &localBridge{tr: tr}
	gate := &e2eGate{messages: make(chan proto.Message, 256)}
	pids := tr.Spawn(
		service.New(configs, reports, bridge, service.Options{Workers: 1, QueueSize: 4, RetryDelay: 20 * time.Millisecond, RetryAttempts: 5}),
		battleclient.New(oneNode{}, bridge, 10, 11),
		gate,
	)
	t.Cleanup(tr.Shutdown)
	clientPID := pids[1]
	input := &pb.BattleInput{ConfigVersion: "default", Seed: 99, Combatants: []*pb.BattleCombatantInput{
		{InstanceId: "hero-1", UnitConfigId: "hero", Team: pb.BattleTeam_BATTLE_TEAM_ATTACKER, Position: 0},
		{InstanceId: "enemy-1", UnitConfigId: "enemy", Team: pb.BattleTeam_BATTLE_TEAM_DEFENDER, Position: 0},
	}}
	value, err := tr.Request(clientPID, &battleclient.Begin{UID: 42, Input: input}).AwaitTimeout(time.Second)
	if err != nil || value.(battleclient.State).Status != battleclient.CreateUnknown {
		t.Fatalf("begin = %#v, %v", value, err)
	}

	var finished *pb.BattleFinishedNtf
	deadline := time.After(3 * time.Second)
	for finished == nil {
		select {
		case message := <-gate.messages:
			if done, ok := message.(*pb.BattleFinishedNtf); ok {
				finished = done
			}
		case <-deadline:
			t.Fatal("timed out waiting for end-to-end Finished")
		}
	}
	stateValue, err := tr.Request(clientPID, &battleclient.GetState{UID: 42}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	state := stateValue.(battleclient.State)
	if state.Status != battleclient.Finished || state.BattleID != finished.BattleId {
		t.Fatalf("client state = %+v, finished = %s", state, finished.BattleId)
	}

	confirmDeadline := time.Now().Add(time.Second)
	for {
		report, getErr := reports.Get(context.Background(), finished.BattleId)
		if getErr == nil && report.Confirmed {
			break
		}
		if time.Now().After(confirmDeadline) {
			t.Fatalf("report not confirmed: %+v, %v", report, getErr)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

package battle

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/ipb"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

type fixedSelector struct {
	node Node
	err  error
}

func (s fixedSelector) Pick() (Node, error) { return s.node, s.err }

type fakeSender struct {
	mu       sync.Mutex
	startErr error
	starts   int
	confirms int
}

func (s *fakeSender) Start(_ Node, _ *pb.StartBattleReq) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	return s.startErr
}
func (s *fakeSender) Confirm(_ Node, _ *pb.BattleResultConfirmedNtf) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.confirms++
	return nil
}
func (s *fakeSender) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts, s.confirms
}

type gateCapture struct{ messages chan *ipb.PushToMsg }

func (g *gateCapture) Name() string        { return def.GameGate }
func (g *gateCapture) OnInit(tree.Context) {}
func (g *gateCapture) OnStop(tree.Context) {}
func (g *gateCapture) HandleMessage(_ tree.Context, msg interface{}) {
	if push, ok := msg.(*ipb.PushToMsg); ok {
		g.messages <- push
	}
}

func clientRequest(uid uint64) *pb.StartBattleReq {
	return &pb.StartBattleReq{UID: uid, BattleType: pb.BattleType_BATTLE_TYPE_TEST, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
}

func requestState(t *testing.T, tr *tree.Tree, pid tree.PID, uid uint64) State {
	t.Helper()
	value, err := tr.Request(pid, &GetState{UID: uid}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return value.(State)
}

func TestSendFailureReturnsIdle(t *testing.T) {
	sender := &fakeSender{startErr: errors.New("publish failed")}
	tr := tree.NewTree()
	pid := tr.SpawnOne(New(fixedSelector{node: Node{ServerID: 1, NodeID: 2}}, sender, 3, 4))
	t.Cleanup(tr.Shutdown)
	if _, err := tr.Request(pid, &Begin{Request: clientRequest(7)}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("expected publish error")
	}
	if got := requestState(t, tr, pid, 7).Status; got != Idle {
		t.Fatalf("state = %s, want Idle", got)
	}
}

func TestCreateUnknownDedupAndIdempotentResult(t *testing.T) {
	sender := &fakeSender{}
	gate := &gateCapture{messages: make(chan *ipb.PushToMsg, 8)}
	tr := tree.NewTree()
	pids := tr.Spawn(New(fixedSelector{node: Node{ServerID: 1, NodeID: 2}}, sender, 3, 4), gate)
	clientPID := pids[0]
	t.Cleanup(tr.Shutdown)
	request := clientRequest(7)
	value, err := tr.Request(clientPID, &Begin{Request: request}).AwaitTimeout(time.Second)
	if err != nil || value.(State).Status != CreateUnknown {
		t.Fatalf("begin = %#v, %v", value, err)
	}
	if _, err := tr.Request(clientPID, &Begin{Request: request}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("CreateUnknown must reject a blind resend")
	}
	starts, _ := sender.counts()
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
	tr.Send(clientPID, &pb.BattleCreatedNtf{BattleId: "b1", Uid: 7})
	tr.Send(clientPID, &pb.BattleActionNtf{BattleId: "b1", Uid: 7, Event: &pb.BattleEvent{Sequence: 1}})
	tr.Send(clientPID, &pb.BattleActionNtf{BattleId: "b1", Uid: 7, Event: &pb.BattleEvent{Sequence: 1}})
	finished := &pb.BattleFinishedNtf{BattleId: "b1", Uid: 7, Checksum: "sum"}
	tr.Send(clientPID, finished)
	tr.Send(clientPID, finished)
	deadline := time.After(2 * time.Second)
	pushes := 0
	for pushes < 3 {
		select {
		case <-gate.messages:
			pushes++
		case <-deadline:
			t.Fatalf("pushes = %d, want Created + one Action + one Finished", pushes)
		}
	}
	time.Sleep(20 * time.Millisecond)
	select {
	case extra := <-gate.messages:
		t.Fatalf("unexpected duplicate push: %T", extra.Msg)
	default:
	}
	state := requestState(t, tr, clientPID, 7)
	if state.Status != Finished || state.BattleID != "b1" || state.LastSequence != 1 {
		t.Fatalf("state = %+v", state)
	}
	_, confirms := sender.counts()
	if confirms != 2 {
		t.Fatalf("confirms = %d, want duplicate results acknowledged twice", confirms)
	}
}

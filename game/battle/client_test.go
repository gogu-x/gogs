package battle

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
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

func requestState(t *testing.T, pid tree.PID, uid uint64) State {
	t.Helper()
	value, err := tree.Request(pid, &GetState{UID: uid}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return value.(State)
}

func TestSendFailureReturnsIdle(t *testing.T) {
	sender := &fakeSender{startErr: errors.New("publish failed")}
	pid := tree.SpawnOne(New(fixedSelector{node: Node{ServerID: 1, NodeID: 2}}, sender, 3, 4))
	t.Cleanup(func() { tree.Stop(pid) })
	openBattleClient(t, pid)
	if _, err := tree.Request(pid, &Begin{Request: clientRequest(7)}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("expected publish error")
	}
	if got := requestState(t, pid, 7).Status; got != Idle {
		t.Fatalf("state = %s, want Idle", got)
	}
	if starts, _ := sender.counts(); starts != 1 {
		t.Fatalf("发布失败测试没有执行发送: %d", starts)
	}
}

func TestCreateUnknownDedupAndIdempotentResult(t *testing.T) {
	sender := &fakeSender{}
	gate := &gateCapture{messages: make(chan *ipb.PushToMsg, 8)}
	pids := []tree.PID{tree.SpawnOne(New(fixedSelector{node: Node{ServerID: 1, NodeID: 2}}, sender, 3, 4)), tree.SpawnOne(gate)}
	clientPID := pids[0]
	t.Cleanup(func() { tree.Stop(pids[1]); tree.Stop(pids[0]) })
	openBattleClient(t, clientPID)
	request := clientRequest(7)
	value, err := tree.Request(clientPID, &Begin{Request: request}).AwaitTimeout(time.Second)
	if err != nil || value.(State).Status != CreateUnknown {
		t.Fatalf("begin = %#v, %v", value, err)
	}
	if _, err := tree.Request(clientPID, &Begin{Request: request}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("CreateUnknown must reject a blind resend")
	}
	starts, _ := sender.counts()
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
	tree.Send(clientPID, &pb.BattleCreatedNtf{BattleId: "b1", Uid: 7})
	tree.Send(clientPID, &pb.BattleActionNtf{BattleId: "b1", Uid: 7, Event: &pb.BattleEvent{Sequence: 1}})
	tree.Send(clientPID, &pb.BattleActionNtf{BattleId: "b1", Uid: 7, Event: &pb.BattleEvent{Sequence: 1}})
	finished := &pb.BattleFinishedNtf{BattleId: "b1", Uid: 7, Checksum: "sum"}
	tree.Send(clientPID, finished)
	tree.Send(clientPID, finished)
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
	state := requestState(t, clientPID, 7)
	if state.Status != Finished || state.BattleID != "b1" || state.LastSequence != 1 {
		t.Fatalf("state = %+v", state)
	}
	_, confirms := sender.counts()
	if confirms != 2 {
		t.Fatalf("confirms = %d, want duplicate results acknowledged twice", confirms)
	}
}

// openBattleClient 完成开服前置条件，确保测试进入真实创建逻辑。
func openBattleClient(t *testing.T, pid tree.PID) {
	t.Helper()
	if tlog.Log == nil {
		tlog.NewLog(t.TempDir(), 0)
	}
	if _, err := tree.Request(pid, gameevent.NewServerOpened()).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestPrecomputedResultKeepsPlaybackAdmission 验证提前结算和重置都不能绕过演出准入窗口。
func TestPrecomputedResultKeepsPlaybackAdmission(t *testing.T) {
	sender := &fakeSender{}
	gate := &gateCapture{messages: make(chan *ipb.PushToMsg, 8)}
	pids := []tree.PID{tree.SpawnOne(New(fixedSelector{node: Node{ServerID: 1, NodeID: 2}}, sender, 3, 4)), tree.SpawnOne(gate)}
	pid := pids[0]
	t.Cleanup(func() { tree.Stop(pids[1]); tree.Stop(pids[0]) })
	openBattleClient(t, pid)
	if _, err := tree.Request(pid, &Begin{Request: clientRequest(7)}).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	tree.Send(pid, &pb.BattleCreatedNtf{BattleId: "fast", Uid: 7, TickDurationMs: 60000})
	tree.Send(pid, &pb.BattleActionNtf{BattleId: "fast", Uid: 7, Event: &pb.BattleEvent{Sequence: 1, Tick: 10, EndTick: 20}})
	tree.Send(pid, &pb.BattleFinishedNtf{BattleId: "fast", Uid: 7})
	if state := requestState(t, pid, 7); state.Status != Finished {
		t.Fatalf("计算结果未立即处理: %+v", state)
	}
	if _, err := tree.Request(pid, &Begin{Request: clientRequest(7)}).AwaitTimeout(time.Second); !errors.Is(err, ErrInProgress) {
		t.Fatalf("提前开启下一场: %v", err)
	}
	if _, err := tree.Request(pid, &Reset{UID: 7}).AwaitTimeout(time.Second); !errors.Is(err, ErrInProgress) {
		t.Fatalf("通过重置绕过演出窗口: %v", err)
	}
	if starts, _ := sender.counts(); starts != 1 {
		t.Fatalf("重复创建战斗: %d", starts)
	}
}

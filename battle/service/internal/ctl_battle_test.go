package internal

import (
	"context"
	"fmt"
	"sync"
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
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), WindupTicks: 3, RecoveryTicks: 3, Effects: []*cspb.TypIDVal{{Typ: "battle_damage", Pro: 1000}}}},
		nil,
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 40, Attack: 10, Defense: 2, Speed: 8, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*cspb.TypIDVal{{Typ: "battle_monster", Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_TOWER), ATBThreshold: 100, MaxActions: 20, TickDurationMS: 100, CritMultiplierPermille: 1500}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// activeBattleQuery 仅用于通过管理器邮箱读取活跃登记。
type activeBattleQuery struct{ battleID string }

// observedServer 保证测试不从其他协程直接读取管理器状态。
type observedServer struct{ *Server }

func (s *observedServer) HandleMessage(ctx tree.Context, message interface{}) {
	if query, ok := message.(*activeBattleQuery); ok {
		ctx.Response(s.active[query.battleID], nil)
		return
	}
	s.Server.HandleMessage(ctx, message)
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
	managerPID := treeSystem.SpawnOne(&observedServer{Server: server})
	t.Cleanup(treeSystem.Shutdown)
	request := &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
	value, err := treeSystem.Request(managerPID, request).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ack := value.(*pb.StartBattleAck)
	if ack.GetBattleId() == "" {
		t.Fatal("missing generated battle id")
	}
	value, err = treeSystem.Request(managerPID, &activeBattleQuery{battleID: ack.GetBattleId()}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	actorPID := value.(tree.PID)
	if actorPID.ID == 0 {
		t.Fatal("战斗未登记")
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
		value, err = treeSystem.Request(managerPID, &activeBattleQuery{battleID: ack.GetBattleId()}).AwaitTimeout(time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if value.(tree.PID).ID == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("战斗登记未清理")
}

// TestBattleComputesBeforeDelivery 验证先算完并保存，再有序发送完整时间轴。
func TestBattleComputesBeforeDelivery(t *testing.T) {
	MainTest(t)
	// 将一帧设为一分钟，旧的真实时间推进方式无法在测试期限内完成。
	glconf.GetBattleRuleCfg(int32(pb.BattleType_BATTLE_TYPE_TOWER)).TickDurationMS = 60000
	server := New()

	var mu sync.Mutex
	type arrival struct {
		tick     int64
		sequence uint64
	}
	// 投递顺序：Action 记 Sequence，Finished 记哨兵。用顺序而非时间戳判断先后，
	// 因为同一毫秒内两次 Notify 的时间戳可能相等。
	const finishedMarker = ^uint64(0)
	var actions []arrival
	var order []uint64
	var badSource []string
	var createdUnits []*pb.BattleUnitResult
	var finishedBattleType pb.BattleType
	var reportMissing bool
	server.notifier = battle.NotifyFunc(func(source battle.Source, message proto.Message) error {
		mu.Lock()
		defer mu.Unlock()
		// 推送目标必须是真实节点：流式推送发生在战报落库之前，早期版本这里会拿到
		// Source{0,0}，事件被投到不存在的 0 号节点后静默丢弃（客户端只收到 Created）。
		if source.ServerID == 0 || source.NodeID == 0 {
			badSource = append(badSource, fmt.Sprintf("%T", message))
		}
		switch value := message.(type) {
		case *pb.BattleActionNtf:
			event := value.GetEvent()
			actions = append(actions, arrival{tick: event.GetTick(), sequence: event.GetSequence()})
			order = append(order, event.GetSequence())
		case *pb.BattleCreatedNtf:
			// 参战名单必须随 Created 下发：否则客户端要等某个单位先行动才建出它，
			// 表现为"进入战斗时对面空无一物"。
			createdUnits = value.GetUnits()
			if report, err := server.reports.Get(context.Background(), value.GetBattleId()); err != nil || len(report.Result.Events) == 0 {
				reportMissing = true
			}
		case *pb.BattleFinishedNtf:
			order = append(order, finishedMarker)
			finishedBattleType = value.GetBattleType()
		}
		return nil
	})

	treeSystem := tree.NewTree()
	managerPID := treeSystem.SpawnOne(server)
	t.Cleanup(treeSystem.Shutdown)
	request := &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
	if _, err := treeSystem.Request(managerPID, request).AwaitTimeout(time.Second); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		done := len(order) > 0 && order[len(order)-1] == finishedMarker
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("battle did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(badSource) > 0 {
		t.Fatalf("推送目标为 0 号节点,消息会被静默丢弃: %v", badSource)
	}
	if reportMissing {
		t.Fatal("创建通知发出前未保存完整战报")
	}
	if len(createdUnits) < 2 {
		t.Fatalf("Created 未携带参战名单: %d 个单位", len(createdUnits))
	}
	// 结算结果必须回带战斗类型：Game 要按类型分发奖励，光看 outcome 判断不出来。
	if finishedBattleType != pb.BattleType_BATTLE_TYPE_TOWER {
		t.Fatalf("Finished 未携带战斗类型: %v", finishedBattleType)
	}
	for _, unit := range createdUnits {
		if unit.GetInstanceId() == "" || unit.GetMaxHp() <= 0 || unit.GetHp() != unit.GetMaxHp() || !unit.GetAlive() {
			t.Fatalf("Created 名单里的单位不完整: %#v", unit)
		}
	}
	if len(actions) < 2 {
		t.Fatalf("Action 事件太少,无法判断是否流式: %d", len(actions))
	}
	for i := 1; i < len(actions); i++ {
		if actions[i].sequence <= actions[i-1].sequence {
			t.Fatalf("Sequence 非递增: %d -> %d", actions[i-1].sequence, actions[i].sequence)
		}
		if actions[i].tick < actions[i-1].tick {
			t.Fatalf("tick 回退: %d -> %d", actions[i-1].tick, actions[i].tick)
		}
	}
	// Finished 必须恰好投递一次,且严格在所有 Action 之后。
	// 用投递顺序而非时间戳判断 —— 同一次 Notify 的时间戳可能落在同一毫秒。
	finishedCount := 0
	for i, seq := range order {
		if seq != finishedMarker {
			continue
		}
		finishedCount++
		if i != len(order)-1 {
			t.Fatalf("Finished 不是最后一次投递: index=%d total=%d", i, len(order))
		}
	}
	if finishedCount != 1 {
		t.Fatalf("Finished 投递次数 = %d,期望 1", finishedCount)
	}
	if actions[len(actions)-1].tick <= actions[0].tick {
		t.Fatal("快速计算丢失了逻辑时间跨度")
	}
}

// TestSlowDeliveryDoesNotBlockManager 验证单场计算后的慢投递不会阻塞管理器创建其他战斗。
func TestSlowDeliveryDoesNotBlockManager(t *testing.T) {
	MainTest(t)
	server := New()
	blocked := make(chan struct{}, 2)
	release := make(chan struct{})
	server.notifier = battle.NotifyFunc(func(_ battle.Source, message proto.Message) error {
		if _, ok := message.(*pb.BattleCreatedNtf); ok {
			blocked <- struct{}{}
			<-release
		}
		return nil
	})
	system := tree.NewTree()
	manager := system.SpawnOne(server)
	t.Cleanup(func() {
		close(release)
		system.Shutdown()
	})
	for uid := uint64(1); uid <= 2; uid++ {
		request := &pb.StartBattleReq{
			UID: uid, ServerID: 1, SourceNodeId: 2,
			BattleType:    pb.BattleType_BATTLE_TYPE_TOWER,
			AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1,
		}
		if _, err := system.Request(manager, request).AwaitTimeout(time.Second); err != nil {
			t.Fatalf("管理器被单场投递阻塞: %v", err)
		}
		select {
		case <-blocked:
		case <-time.After(time.Second):
			t.Fatal("战斗没有独立进入投递阶段")
		}
	}
}

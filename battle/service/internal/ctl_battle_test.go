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

// MainTest 初始化服务测试所需的日志和战斗配置。
func MainTest(t *testing.T) {
	t.Helper()
	tlog.NewLog(conf.LogPath, 0)
	err := glconf.SetBattleConfigsForTest(
		[]*glconf.BattleRoleCfg{{CfgID: 1, MaxHP: 100, Attack: 20, Defense: 5, Speed: 10, DefaultPosition: 1, BasicSkillID: 1}},
		[]*glconf.BattleSkillCfg{{CfgID: 1, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE), Effects: []*cspb.TypIDVal{{Typ: "battle_damage", Pro: 1000}}}},
		nil,
		[]*glconf.BattleMonsterCfg{{CfgID: 1, MaxHP: 40, Attack: 10, Defense: 2, Speed: 8, BasicSkillID: 1}},
		[]*glconf.BattleMonsterGroupCfg{{CfgID: 1, Members: []*cspb.TypIDVal{{Typ: "battle_monster", Id: 1, Val: 1}}}},
		[]*glconf.BattleRuleCfg{{BattleType: int32(pb.BattleType_BATTLE_TYPE_TOWER), ATBThreshold: 100, CritMultiplierPermille: 1500}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// activeBattleQuery 仅用于通过管理器邮箱读取活跃登记。
type activeBattleQuery struct{ battleID string }

// observedServer 保证测试不从其他协程直接读取管理器状态。
type observedServer struct{ *Server }

// TestCreateBattleSpawnsAndTracksActor 验证管理器创建战斗并跟踪实例的生命周期。
func TestCreateBattleSpawnsAndTracksActor(t *testing.T) {
	MainTest(t)
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

	managerPID := spawnTestServer(t, server, &observedServer{Server: server})
	request := &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
	value, err := tree.Request(managerPID, request).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ack := value.(*pb.StartBattleAck)
	if ack.GetBattleId() == "" {
		t.Fatal("missing generated battle id")
	}
	value, err = tree.Request(managerPID, &activeBattleQuery{battleID: ack.GetBattleId()}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	actorPID := value.(tree.PID)
	t.Cleanup(func() { tree.Stop(actorPID) })
	if actorPID.ID == 0 {
		t.Fatal("战斗未登记")
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("battle did not finish")
	}
	tree.Send(managerPID, &battle.ActorStopped{BattleID: ack.GetBattleId(), PID: actorPID})
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		value, err = tree.Request(managerPID, &activeBattleQuery{battleID: ack.GetBattleId()}).AwaitTimeout(time.Second)
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

// TestBattleStreamsBeforeFinalReport 验证开场推送快照，过程事件即时发送，结束通知前保存战报。
func TestBattleStreamsBeforeFinalReport(t *testing.T) {
	MainTest(t)
	server := New()

	var mu sync.Mutex
	type arrival struct {
		elapsedMS int64
		sequence  uint64
	}
	// 投递顺序：Action 记 Sequence，Finished 记哨兵。用顺序而非时间戳判断先后，
	// 因为同一毫秒内两次 Notify 的时间戳可能相等。
	const finishedMarker = ^uint64(0)
	var actions []arrival
	var order []uint64
	var badSource []string
	var createdUnits []*pb.BattleUnitResult
	var finishedBattleType pb.BattleType
	var reportMissing, reportAtCreation bool
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
			actions = append(actions, arrival{elapsedMS: event.GetElapsedMs(), sequence: event.GetSequence()})
			order = append(order, event.GetSequence())
		case *pb.BattleCreatedNtf:
			// 参战名单必须随 Created 下发：否则客户端要等某个单位先行动才建出它，
			// 表现为"进入战斗时对面空无一物"。
			createdUnits = value.GetUnits()
			_, err := server.reports.Get(context.Background(), value.GetBattleId())
			reportAtCreation = err == nil
		case *pb.BattleFinishedNtf:
			report, err := server.reports.Get(context.Background(), value.GetBattleId())
			reportMissing = err != nil || len(report.Result.Events) == 0
			order = append(order, finishedMarker)
			finishedBattleType = value.GetBattleType()
		}
		return nil
	})

	managerPID := spawnTestServer(t, server, server)
	request := &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1}
	if _, err := tree.Request(managerPID, request).AwaitTimeout(time.Second); err != nil {
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
	if reportAtCreation || reportMissing {
		t.Fatal("应在开场之后、结束通知之前生成战报")
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
		if actions[i].elapsedMS < actions[i-1].elapsedMS {
			t.Fatalf("elapsedMS 回退: %d -> %d", actions[i-1].elapsedMS, actions[i].elapsedMS)
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
	if actions[len(actions)-1].elapsedMS <= actions[0].elapsedMS {
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

	manager := spawnTestServer(t, server, server)
	t.Cleanup(func() {
		close(release)
	})
	for uid := uint64(1); uid <= 2; uid++ {
		request := &pb.StartBattleReq{
			UID: uid, ServerID: 1, SourceNodeId: 2,
			BattleType:    pb.BattleType_BATTLE_TYPE_TOWER,
			AttackerRoles: []*pb.Role{{RoleId: "role-1", RoleConfigId: 1}}, MonsterGroupConfigId: 1,
		}
		if _, err := tree.Request(manager, request).AwaitTimeout(time.Second); err != nil {
			t.Fatalf("管理器被单场投递阻塞: %v", err)
		}
		select {
		case <-blocked:
		case <-time.After(time.Second):
			t.Fatal("战斗没有独立进入投递阶段")
		}
	}
}

// spawnTestServer 使用全局入口创建管理器，并在测试结束时先回收其战斗 Actor。
func spawnTestServer(t *testing.T, server *Server, actor tree.Actor) tree.PID {
	t.Helper()
	pid := tree.SpawnOne(actor)
	t.Cleanup(func() {
		children := make(chan []tree.PID, 1)
		if tree.SendCallback(pid, func(tree.Context, interface{}, error) {
			pids := make([]tree.PID, 0, len(server.active))
			for _, child := range server.active {
				pids = append(pids, child)
			}
			children <- pids
		}, nil, nil) {
			select {
			case pids := <-children:
				for _, child := range pids {
					tree.Stop(child)
				}
			case <-time.After(time.Second):
				t.Error("管理器未返回待回收 Actor")
			}
		}
		tree.Stop(pid)
	})
	return pid
}

// TestInteractiveRobotAndEquippedSkill 从创建入口验证机器人主动普攻和玩家即时法宝结算。
func TestInteractiveRobotAndEquippedSkill(t *testing.T) {
	MainTest(t)
	const firstDelay = 23 // 测试怪物的首次决策时间。
	monster := glconf.GetBattleMonsterCfg(1)
	monster.AIFirstDelayMS = firstDelay
	monster.AICastRules = []*cspb.TypIDVal{{Typ: "always", Id: 1}}
	basic := glconf.GetBattleSkillCfg(1)
	if err := glconf.SetBattleConfigsForTest(
		glconf.GetAllBattleRoleCfg(), []*glconf.BattleSkillCfg{basic, {CfgID: 2, TargetRule: basic.TargetRule, Effects: []*cspb.TypIDVal{{Typ: "battle_damage", Pro: 100000}}}},
		[]*glconf.BattleEquipCfg{{CfgID: 10, ActiveSkillID: 2}}, glconf.GetAllBattleMonsterCfg(), glconf.GetAllBattleMonsterGroupCfg(), glconf.GetAllBattleRuleCfg(),
	); err != nil {
		t.Fatal(err)
	}
	server := New()
	events := make(chan proto.Message, 256)
	server.notifier = battle.NotifyFunc(func(_ battle.Source, msg proto.Message) error { events <- msg; return nil })
	manager := spawnTestServer(t, server, &observedServer{Server: server})
	role := &pb.Role{RoleId: "player", RoleConfigId: 1, Equips: []*pb.RoleEquip{{EquipConfigId: 10}}}
	value, err := tree.Request(manager, &pb.StartBattleReq{UID: 7, ServerID: 1, SourceNodeId: 2, BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{role}, MonsterGroupConfigId: 1}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	id := value.(*pb.StartBattleAck).GetBattleId()
	value, err = tree.Request(manager, &activeBattleQuery{battleID: id}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pid := value.(tree.PID)
	deadline := time.After(2 * time.Second)
	var created, sent, accepted bool
	for {
		select {
		case <-deadline:
			t.Fatal("机器人或法宝结算超时")
		case msg := <-events:
			switch event := msg.(type) {
			case *pb.BattleCreatedNtf:
				controls := event.GetControls()
				if len(controls) != 1 || len(controls[0].GetActiveSkills()) != 1 || controls[0].GetActiveSkills()[0] != "2" {
					t.Fatalf("法宝未随装备生效 %v", controls)
				}
				created = true
			case *pb.BattleActionNtf:
				if !sent && event.GetEvent().GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetEvent().GetTargetId() == "player" {
					if event.GetEvent().GetElapsedMs() < firstDelay {
						t.Fatal("怪物早于配置时间出手")
					}
					sent = tree.Send(pid, &pb.BattleInputReq{UID: 7, BattleId: id, UnitId: "player", SkillId: "2", Sequence: 1})
				}
			case *pb.BattleInputAck:
				accepted = event.GetCode() == cspb.ErrCode_OK
			case *pb.BattleFinishedNtf:
				if !created || !sent || !accepted || event.GetOutcome() != pb.BattleOutcome_BATTLE_OUTCOME_ATTACKER_WIN {
					t.Fatalf("交互战斗结果异常 %v", event)
				}
				return
			}
		}
	}
}

// TestSharedBossRoom 验证按名单立即开场、中途加入血量快照、控制权及逐玩家确认。
func TestSharedBossRoom(t *testing.T) {
	MainTest(t)
	// 将怪物第一次决策推迟，让测试能精确控制每一次扣血和加入顺序。
	glconf.GetBattleMonsterCfg(1).AIFirstDelayMS = 60000
	server := New()
	events := make(chan proto.Message, 128)
	server.notifier = battle.NotifyFunc(func(source battle.Source, msg proto.Message) error {
		// 三个玩家来自不同 Game 节点。
		if routed, ok := msg.(interface{ GetUid() uint64 }); ok && int(routed.GetUid()) != source.NodeID {
			t.Errorf("回投错节点 %v %v", source, msg)
		}
		events <- msg
		return nil
	})
	manager := spawnTestServer(t, server, &observedServer{Server: server})
	request := func(uid uint64, id string) *pb.StartBattleReq {
		return &pb.StartBattleReq{UID: uid, ServerID: 1, SourceNodeId: uint32(uid), BattleType: pb.BattleType_BATTLE_TYPE_TOWER, AttackerRoles: []*pb.Role{{RoleId: "role", RoleConfigId: 1}}, MonsterGroupConfigId: 1, JoinBattleId: id}
	}
	next := func() proto.Message {
		t.Helper()
		select {
		case msg := <-events:
			return msg
		case <-time.After(time.Second):
			t.Fatal("未收到战斗消息")
			return nil
		}
	}
	host := request(1, "")
	host.AllowJoin = true
	host.InitialMembers = []*pb.InitialBattleMember{{Uid: 2, Role: request(2, "").GetAttackerRoles()[0], ServerId: 1, NodeId: 2}}
	value, err := tree.Request(manager, host).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	id := value.(*pb.StartBattleAck).GetBattleId()
	controls := map[uint64]string{}
	for len(controls) < 2 {
		created, ok := next().(*pb.BattleCreatedNtf)
		if !ok || len(created.GetControls()) != 1 || len(created.GetUnits()) != 3 {
			t.Fatal("初始名单或控制权错误", created)
		}
		controls[created.GetUid()] = created.GetControls()[0].GetUnitId()
	}
	if controls[1] == controls[2] {
		t.Fatal("角色实例冲突")
	}
	v, err := tree.Request(manager, &activeBattleQuery{battleID: id}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pid := v.(tree.PID)
	act := func(uid, seq uint64, unit string) {
		tree.Send(pid, &pb.BattleInputReq{UID: uid, BattleId: id, UnitId: unit, SkillId: "1", Sequence: seq})
	}
	act(2, 1, controls[1])
	if ack := next().(*pb.BattleInputAck); ack.GetCode() == cspb.ErrCode_OK {
		t.Fatal("队友越权操作成功")
	}
	act(1, 1, controls[1])
	hits := map[uint64]int64{}
	for len(hits) < 2 {
		if msg, ok := next().(*pb.BattleActionNtf); ok && msg.GetEvent().GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE {
			hits[msg.GetUid()] = msg.GetEvent().GetHpAfter()
		}
	}
	if hits[1] != hits[2] || hits[1] <= 0 || hits[1] >= 40 {
		t.Fatalf("两人血量不同 %v", hits)
	}
	tree.Send(manager, request(3, id))
	for controls[3] == "" {
		created, ok := next().(*pb.BattleCreatedNtf)
		if !ok {
			continue
		}
		if created.GetUid() != 3 || len(created.GetControls()) != 1 || len(created.GetUnits()) != 4 || created.GetSnapshotSequence() == 0 {
			t.Fatalf("晚加入快照错误 %v", created)
		}
		for _, u := range created.GetUnits() {
			if u.GetTeam() == pb.BattleTeam_BATTLE_TEAM_DEFENDER && u.GetHp() != hits[1] {
				t.Fatal("Boss 血量被重置")
			}
		}
		controls[3] = created.GetControls()[0].GetUnitId()
	}
	act(2, 1, controls[2])
	act(3, 1, controls[3])
	results := map[uint64]string{}
	for len(results) < 3 {
		if msg, ok := next().(*pb.BattleFinishedNtf); ok {
			results[msg.GetUid()] = msg.GetChecksum()
		}
	}
	if results[1] == "" || results[1] != results[2] || results[1] != results[3] {
		t.Fatalf("结算分歧 %v", results)
	}
	for uid := uint64(1); uid <= 3; uid++ {
		tree.Send(pid, &pb.BattleResultConfirmedNtf{BattleId: id, Uid: uid})
		if uid < 3 {
			// 在同场邮箱内设置屏障，确保查询发生在本次确认之后。
			barrier := make(chan struct{})
			tree.SendCallback(pid, func(tree.Context, interface{}, error) { close(barrier) }, nil, nil)
			select {
			case <-barrier:
			case <-time.After(time.Second):
				t.Fatal("提前回收")
			}
			report, err := server.reports.Get(context.Background(), id)
			if err != nil || report.Confirmed {
				t.Fatal("提前确认整场")
			}
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reports, _ := server.reports.ListByPlayer(context.Background(), 3, 1)
		if len(reports) == 1 && reports[0].Confirmed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("未完成全员确认")
}

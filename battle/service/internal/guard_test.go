package internal

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// TestSharedGuardAndCounter 验证机器人预告、定时命中和两位玩家收到的格挡与反击结果一致。
func TestSharedGuardAndCounter(t *testing.T) {
	MainTest(t)
	monster := glconf.GetBattleMonsterCfg(1)
	monster.ActiveSkillIDs = []int32{3}
	monster.AISkillSequence = []int32{3}
	monster.AIFirstDelayMS = 40
	heavy := &glconf.BattleSkillCfg{
		CfgID: 3, TargetRule: int32(pb.TargetRule_TARGET_RULE_ENEMY_SINGLE),
		Effects: []*common.TypIDVal{{Typ: "battle_damage", Val: 20, Pro: 1000}},
	}
	if err := glconf.SetBattleConfigsForTest(
		glconf.GetAllBattleRoleCfg(), []*glconf.BattleSkillCfg{glconf.GetBattleSkillCfg(1), heavy},
		nil, glconf.GetAllBattleMonsterCfg(), glconf.GetAllBattleMonsterGroupCfg(), glconf.GetAllBattleRuleCfg(),
	); err != nil {
		t.Fatal(err)
	}
	server := New()
	messages := make(chan proto.Message, 256)
	server.notifier = battle.NotifyFunc(func(_ battle.Source, msg proto.Message) error {
		messages <- proto.Clone(msg)
		return nil
	})
	manager := spawnTestServer(t, server, &observedServer{Server: server})
	role := &pb.Role{RoleId: "player", RoleConfigId: 1}
	req := &pb.StartBattleReq{
		UID: 1, ServerID: 1, SourceNodeId: 1, BattleType: pb.BattleType_BATTLE_TYPE_TOWER,
		AttackerRoles: []*pb.Role{role}, MonsterGroupConfigId: 1,
		InitialMembers: []*pb.InitialBattleMember{{Uid: 2, ServerId: 1, NodeId: 2, Role: role}},
	}
	started, err := tree.Request(manager, req).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	id := started.(*pb.StartBattleAck).GetBattleId()
	value, err := tree.Request(manager, &activeBattleQuery{battleID: id}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pid := value.(tree.PID)
	owners := make(map[string]uint64)
	guards, counters := make(map[uint64]*pb.BattleEvent), make(map[uint64]*pb.BattleEvent)
	var guardAt <-chan time.Time
	var locked string
	act := func(skill string, seq uint64) {
		if !tree.Send(pid, &pb.BattleInputReq{UID: owners[locked], BattleId: id, UnitId: locked, Sequence: seq, SkillId: skill}) {
			t.Fatal("未能提交玩家动作", skill)
		}
	}
	deadline := time.After(5 * time.Second)
	for len(counters) < 2 {
		select {
		case <-deadline:
			t.Fatalf("未完成共享格挡与反击，目标 %s，格挡 %d 次，反击 %d 次", locked, len(guards), len(counters))
		case <-guardAt:
			guardAt = nil
			act("guard", 1)
		case msg := <-messages:
			switch ntf := msg.(type) {
			case *pb.BattleCreatedNtf:
				controls := ntf.GetControls()
				if len(controls) != 1 || !controls[0].GetCanGuard() {
					t.Fatal("首发或队友没有架势权限", ntf)
				}
				owners[controls[0].GetUnitId()] = ntf.GetUid()
			case *pb.BattleActionNtf:
				event, uid := ntf.GetEvent(), ntf.GetUid()
				switch {
				case event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED && event.GetSkillId() == "3" && locked == "":
					if event.GetImpactMs()-event.GetStartMs() != 1500 {
						t.Fatal("重击预告时间错误", event)
					}
					locked = event.GetTargetId()
					guardAt = time.After(1100 * time.Millisecond)
				case event.GetGuarded():
					if event.GetTargetId() != locked {
						t.Fatal("实际格挡者与锁定目标不符", event)
					}
					guards[uid] = event
					if uid == owners[locked] {
						act("1", 2)
					}
				case event.GetCounter():
					counters[uid] = event
				}
			}
		}
	}
	if len(guards) != 2 || !proto.Equal(guards[1], guards[2]) || !proto.Equal(counters[1], counters[2]) {
		t.Fatal("两名玩家收到的伤害或状态不同", guards, counters)
	}
}

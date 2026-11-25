package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal/battleconfig"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// Member 保存玩家的回投节点和可操作单位，战报重投也按此名单分发。
type Member struct {
	UID     uint64   // 玩家编号。
	Source  Source   // 归属 Game 节点。
	UnitIDs []string // 本场可控的单位实例编号。
}

func (a *BattleActor) member(uid uint64) *Member {
	for i := range a.members {
		if a.members[i].UID == uid {
			return &a.members[i]
		}
	}
	return nil
}

func (a *BattleActor) initMembers() {
	member := Member{UID: a.p.UID, Source: a.p.Source}
	for _, role := range a.p.Request.GetAttackerRoles() {
		member.UnitIDs = append(member.UnitIDs, role.GetRoleId())
	}
	a.members = []Member{member}
}

func (a *BattleActor) notifyAdmissionFailure(source Source, uid uint64, code common.ErrCode) {
	msg := &pb.BattleAdmissionFailedNtf{BattleId: a.p.BattleID, Uid: uid, Code: code}
	if err := a.p.Notifier.Notify(source, msg); err != nil {
		tlog.Log.Warn("[战斗/notifyAdmissionFailure] 入场失败通知投递失败, pid=%v err=%v", uid, err)
	}
}

// failCreation 通知全部初始成员释放本次创建状态。
func (a *BattleActor) failCreation() {
	a.notifyAdmissionFailure(a.p.Source, a.p.UID, common.ErrCode_BATTLE_CREATE_FAILED)
	for _, m := range a.p.Request.GetInitialMembers() {
		a.notifyAdmissionFailure(Source{ServerID: int(m.GetServerId()), NodeID: int(m.GetNodeId())}, m.GetUid(), common.ErrCode_BATTLE_CREATE_FAILED)
	}
}

// join 仅在本场邮箱内变更名单，校验玩法与玩家当前层数后才接入角色。
func (a *BattleActor) join(ctx tree.Context, req *pb.StartBattleReq) {
	source := Source{ServerID: int(req.GetServerID()), NodeID: int(req.GetSourceNodeId())}
	uid := req.GetUID()
	reject := func() { a.notifyAdmissionFailure(source, uid, common.ErrCode_INVALID_BATTLE_REQUEST) }
	if req.GetJoinBattleId() != a.p.BattleID || !a.p.Request.GetAllowJoin() || a.live == nil || a.p.Battle == nil || a.released {
		reject()
		return
	}
	if m := a.member(uid); m != nil {
		if source != m.Source {
			reject()
			return
		}
		if a.advanceLive(ctx) {
			a.sendSnapshot(*m, true)
		}
		return
	}
	if len(req.GetAttackerRoles()) != 1 || req.GetBattleType() != a.p.Request.GetBattleType() || req.GetMonsterGroupConfigId() != a.p.Request.GetMonsterGroupConfigId() {
		reject()
		return
	}
	if !a.advanceLive(ctx) {
		reject()
		return
	}
	if err := a.addMember(&pb.InitialBattleMember{Uid: uid, Role: req.GetAttackerRoles()[0], ServerId: req.GetServerID(), NodeId: req.GetSourceNodeId()}, true); err != nil {
		reject()
		return
	}
	// 已在场的玩家按序收到新增单位事件；新玩家以当前血量快照为起点。
	events := a.p.Battle.Flush()
	for _, m := range a.members[:len(a.members)-1] {
		a.pushMemberEvents(m, events)
	}
	a.sendSnapshot(a.members[len(a.members)-1], true)
	a.scheduleStatus()
}

// sendSnapshot 下发当前全场血量，但只交付收件人自己的操作权限。
func (a *BattleActor) sendSnapshot(m Member, baseline bool) {
	b := a.p.Battle
	msg := &pb.BattleCreatedNtf{BattleId: a.p.BattleID, Uid: m.UID, TickDurationMs: b.TickDurationMS(), Units: b.UnitResults()}
	cursor, tick := b.SnapshotCursor()
	msg.CurrentTick = tick
	if baseline {
		msg.SnapshotSequence = cursor
		msg.Statuses = b.StatusSnapshot()
	}
	for _, control := range b.Controls() {
		if m.owns(control.GetUnitId()) {
			msg.Controls = append(msg.Controls, control)
		}
	}
	if err := a.p.Notifier.Notify(m.Source, msg); err != nil {
		tlog.Log.Warn("[战斗/sendSnapshot] 快照投递失败, pid=%v err=%v", m.UID, err)
	}
}

func (m Member) owns(id string) bool {
	for _, owned := range m.UnitIDs {
		if owned == id {
			return true
		}
	}
	return false
}

func (a *BattleActor) pushMemberEvents(m Member, events []*pb.BattleEvent) {
	for _, event := range events {
		msg := &pb.BattleActionNtf{BattleId: a.p.BattleID, Uid: m.UID, Event: event}
		if err := a.p.Notifier.Notify(m.Source, msg); err != nil {
			tlog.Log.Warn("[战斗/pushMemberEvents] 过程投递失败, pid=%v err=%v", m.UID, err)
		}
	}
}

// addMember 从权威角色快照创建单位，初始成员与增援使用同一套归属和校验。
func (a *BattleActor) addMember(m *pb.InitialBattleMember, started bool) error {
	uid := m.GetUid()
	if uid == 0 || m.GetRole() == nil || m.GetServerId() == 0 || m.GetNodeId() == 0 || a.member(uid) != nil {
		return fmt.Errorf("无效参战成员 %d", uid)
	}
	compiler := a.p.Compiler
	if compiler == nil {
		compiler = battleconfig.NewDefault()
	}
	config, err := compiler.BuildJoinedRole(m.GetRole())
	if err != nil {
		return err
	}
	config.OwnerUID = uid
	config.InstanceID = fmt.Sprintf("player-%d-%s", uid, config.InstanceID)
	if err := a.p.Battle.AddAttacker(config, started); err != nil {
		return err
	}
	a.members = append(a.members, Member{UID: uid, Source: Source{ServerID: int(m.GetServerId()), NodeID: int(m.GetNodeId())}, UnitIDs: []string{config.InstanceID}})
	return nil
}

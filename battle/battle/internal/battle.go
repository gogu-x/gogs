package internal

import (
	"context"
	"errors"

	"github.com/gogu-x/gogs/battle/battle/internal/battleconfig"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"

	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// battleMailboxSize 是 actor 邮箱容量。时间轮回调与普通用户消息共用这个邮箱，
	// 而 BattleResultConfirmedNtf 是从共享的 Nats actor goroutine 投递进来的
	// （投递是阻塞的），邮箱被填满会连带卡住整个进程的 NATS 路由，所以留足余量。
	battleMailboxSize = 256
)

// BattleActor 独立完成整场计算、战报保存、有序推送和结果确认。
// 注册名 = def.BattleActorName(battleID)，跨进程可被 NATS 按 TaggerName
// 直达（如 BattleResultConfirmedNtf 确认）。
type BattleActor struct {
	offers    map[string][]*pb.BattleGrant // 单位尚未领取的候选，保留发放顺序。
	roles     map[string]*pb.Role          // 参战角色最近一次权威编译输入。
	router    tree.Router
	members   []Member
	confirmed map[uint64]bool
	live      *liveBattle
	p         Params
	report    Report // 已保存、等待投递或确认的完整战报。
	attempts  int
	timer     *timer.TimeWheel
	released  bool
	self      tree.PID
}

// NewBattleActor 按创建参数构造单场战斗的 Actor。
func NewBattleActor(p Params) *BattleActor {
	return &BattleActor{p: p}
}

// Name 返回 Actor 的注册名称。
func (a *BattleActor) Name() string { return def.BattleActorName(a.p.BattleID) }

// MailboxSize 返回战斗消息邮箱的容量。
func (a *BattleActor) MailboxSize() int { return battleMailboxSize }

// OnInit 初始化消息路由及运行所需的资源。
func (a *BattleActor) OnInit(ctx tree.Context) {
	a.self = ctx.Self()
	a.registerMessages()
	a.initTimer()
	// SpawnOne 等待初始化结束；计算放入邮箱，避免阻塞管理器创建其他战斗。
	tree.Send(a.self, &beginBattle{})
}

// OnStop 停止当前 Actor 的调度资源。
func (a *BattleActor) OnStop(_ tree.Context) { a.stopRobots(); a.stopTimer() }

// HandleMessage 将邮箱消息交给已注册的路由。
func (a *BattleActor) HandleMessage(ctx tree.Context, msg interface{}) {
	a.router.Route(ctx, msg)
}

// begin 创建即时战斗，或重投已有战报。
func (a *BattleActor) begin(ctx tree.Context) {
	if a.p.Mode == ModeRun && a.p.Battle == nil {
		compiler := a.p.Compiler
		if compiler == nil {
			compiler = battleconfig.NewDefault()
		}
		setup, err := compiler.Compile(a.p.BattleID, a.p.Seed, a.p.Request)
		if err == nil {
			a.p.Battle, err = engine.NewBattle(setup)
		}
		if err != nil {
			tlog.Log.Error("[战斗/创建] 读取配置并创建引擎失败, battleID=%v uid=%v err=%v", a.p.BattleID, a.p.UID, err)
			a.failCreation()
			a.finish(ctx)
			return
		}
	}
	if a.p.Mode == ModeRun {
		a.initMembers()
		for _, member := range a.p.Request.GetInitialMembers() {
			if err := a.addMember(member, false); err != nil {
				a.failCreation()
				a.finish(ctx)
				return
			}
		}
		a.beginLive()
		return
	}

	// ModeRedeliver：复用库中已有战报，但把投递回执目标覆盖为当前请求方。
	r := a.p.Report
	r.UID = a.p.UID
	r.SourceServerID = a.p.Source.ServerID
	r.SourceNodeID = a.p.Source.NodeID
	a.report = r
	// 重投只面向本次指定的收件人。
	a.members = []Member{{UID: a.p.UID, Source: a.p.Source}}
	a.notifyCreated()
	a.pushActions()
	a.pushFinished()
	if a.report.Confirmed {
		// 库中战报已确认：补发完成无需再等确认回执，直接退出。
		a.finish(ctx)
	}
}

// notifyCreated 根据战报向玩法服发送战斗创建通知。
func (a *BattleActor) notifyCreated() {
	created := &pb.BattleCreatedNtf{BattleId: a.p.BattleID, Uid: a.p.UID}
	// 参战名单在创建时即已确定，随 Created 一起下发，客户端进战斗页就能把双方单位摆好。

	created.Units = a.report.InitialUnits
	if err := a.p.Notifier.Notify(a.p.Source, created); err != nil {
		tlog.Log.Warn("battle battle %s: notify Created: %v", a.p.BattleID, err)
	}
}

// pushActions 按逻辑时间和序号发送完整事件，不等待演出时间。
func (a *BattleActor) pushActions() {
	a.pushEvents(a.report.BattleID, a.report.UID, a.report.Result.Events)
}

// pushEvents 保持现有消息格式，逐条发布事件；真实演出节奏由客户端控制。
func (a *BattleActor) pushEvents(battleID string, uid uint64, events []*pb.BattleEvent) {
	if len(events) == 0 {
		return
	}
	source := a.pushSource()
	for _, event := range events {
		if event == nil {
			continue
		}
		a.logEvent(event)
		ntf := &pb.BattleActionNtf{BattleId: battleID, Uid: uid, Event: event}
		if err := a.p.Notifier.Notify(source, ntf); err != nil {
			tlog.Log.Warn("battle battle %s: notify Action %d: %v", a.p.BattleID, event.GetSequence(), err)
		}
	}
}

// logEvent 记录单条战斗事件。
func (a *BattleActor) logEvent(event *pb.BattleEvent) {
	if tlog.Log == nil {
		return
	}
	tlog.Log.Info(
		"[战斗/过程] battleID=%v seq=%v action=%v elapsedMS=%v type=%v actor=%v target=%v skill=%v status=%v amount=%v hp=%v->%v effect=%v statusType=%v critical=%v basic=%v reason=%v outcome=%v",
		a.p.BattleID, event.GetSequence(), event.GetAction(), event.GetElapsedMs(), event.GetType(),
		event.GetActorId(), event.GetTargetId(), event.GetSkillId(), event.GetStatusId(),
		event.GetAmount(), event.GetHpBefore(), event.GetHpAfter(), event.GetEffectType(),
		event.GetStatusType(), event.GetCritical(), event.GetBasicAttack(), event.GetReason(), event.GetOutcome(),
	)
	if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetHpBefore() > 0 && event.GetHpAfter() == 0 {
		tlog.Log.Info("[战斗/死亡] battleID=%v unit=%v killer=%v skill=%v action=%v elapsedMS=%v", a.p.BattleID, event.GetTargetId(), event.GetActorId(), event.GetSkillId(), event.GetAction(), event.GetElapsedMs())
	}
}

// logResult 记录战斗结算结果。
func (a *BattleActor) logResult(result engine.Result) {
	if tlog.Log == nil {
		return
	}
	for _, unit := range result.Units {
		if unit == nil {
			continue
		}
		tlog.Log.Info("[战斗/单位] battleID=%v unit=%v team=%v hp=%v/%v alive=%v", result.BattleID, unit.GetInstanceId(), unit.GetTeam(), unit.GetHp(), unit.GetMaxHp(), unit.GetAlive())
	}
	tlog.Log.Info("[战斗/结算] 自动战斗完成, battleID=%v uid=%v outcome=%v elapsedMS=%v events=%v checksum=%v", result.BattleID, a.p.UID, result.Outcome, result.ElapsedMs, len(result.Events), result.Checksum)
}

// pushSource 返回战报的投递目标，未指定时使用创建请求的来源节点。
func (a *BattleActor) pushSource() Source {
	if a.report.SourceServerID == 0 && a.report.SourceNodeID == 0 {
		return a.p.Source
	}
	return Source{ServerID: a.report.SourceServerID, NodeID: a.report.SourceNodeID}
}

// pushFinished 推送 Finished 并（未确认时）安排重试。
func (a *BattleActor) pushFinished() {
	result := a.report.Result
	members := a.members
	if len(members) == 0 {
		members = []Member{{UID: a.report.UID, Source: a.pushSource()}}
	}
	for _, m := range members {
		if a.confirmed[m.UID] {
			continue
		}
		ntf := &pb.BattleFinishedNtf{BattleId: a.report.BattleID, Uid: m.UID, BattleType: result.BattleType, Outcome: result.Outcome, Units: result.Units, Checksum: result.Checksum}
		if err := a.p.Notifier.Notify(m.Source, ntf); err != nil {
			tlog.Log.Warn("[战斗/pushFinished] 结束通知失败, pid=%v err=%v", m.UID, err)
		}
	}
	a.attempts++
	if !a.report.Confirmed {
		a.scheduleRetry()
	}
}

// retry 检查战报确认状态并处理重投。
func (a *BattleActor) retry(ctx tree.Context) {
	if a.attempts >= a.p.RetryAttempts {
		a.finish(ctx)
		return
	}
	a.pushFinished()
}

// confirm 收到 Game 侧确认（NATS 按 battle 名直达）。幂等落库后退出。
func (a *BattleActor) confirm(ctx tree.Context, msg *pb.BattleResultConfirmedNtf) {
	if msg == nil || msg.BattleId != a.p.BattleID {
		return
	}
	if len(a.members) > 0 {
		if a.member(msg.GetUid()) == nil {
			return
		}
		if a.confirmed == nil {
			a.confirmed = make(map[uint64]bool)
		}
		a.confirmed[msg.GetUid()] = true
		if len(a.confirmed) < len(a.members) {
			return
		}
	}
	if err := a.p.Reports.Confirm(context.Background(), a.p.BattleID); err != nil && !errors.Is(err, ErrReportNotFound) {
		tlog.Log.Warn("battle battle %s: confirm: %v", a.p.BattleID, err)
	}
	a.finish(ctx)
}

// finish 幂等地结束本 battle：通知 manager 清理登记后自己停止。
func (a *BattleActor) finish(ctx tree.Context) {
	if a.released {
		return
	}
	a.released = true
	a.stopTimer()
	if a.managerPID().ID != 0 {
		tree.Send(a.managerPID(), &ActorStopped{BattleID: a.p.BattleID, PID: a.self})
	}
	ctx.Stop()
}

// managerPID 返回管理本场战斗的服务地址。
func (a *BattleActor) managerPID() tree.PID { return a.p.ManagerPID }

// stopTimer 停止本场战斗的定时器。
func (a *BattleActor) stopTimer() {
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	p        Params
	report   Report // 已保存、等待投递或确认的完整战报。
	attempts int
	timer    *timer.TimeWheel
	released bool
	self     tree.PID
}

func NewBattleActor(p Params) *BattleActor {
	return &BattleActor{p: p}
}

func (a *BattleActor) Name() string     { return def.BattleActorName(a.p.BattleID) }
func (a *BattleActor) MailboxSize() int { return battleMailboxSize }

func (a *BattleActor) OnInit(ctx tree.Context) {
	a.self = ctx.Self()
	a.initTimer()
	// SpawnOne 等待初始化结束；计算放入邮箱，避免阻塞管理器创建其他战斗。
	tree.Send(a.self, &beginBattle{})
}

func (a *BattleActor) OnStop(_ tree.Context) { a.stopTimer() }

func (a *BattleActor) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *beginBattle:
		a.begin(ctx)
	case *pb.BattleResultConfirmedNtf:
		a.confirm(ctx, m)
	case *retryTick:
		a.retry(ctx)
	case *RetireBattle:
		a.finish(ctx)
	default:
		ctx.Response(nil, fmt.Errorf("battle battle %s: unsupported message %T", a.p.BattleID, msg))
	}
}

// begin 先完成计算和战报保存，再按创建、过程、结束顺序下发。
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
			a.finish(ctx)
			return
		}
	}
	if a.p.Mode == ModeRun {
		a.runBattle(ctx)
		return
	}

	// ModeRedeliver：复用库中已有战报，但把投递回执目标覆盖为当前请求方。
	r := a.p.Report
	r.UID = a.p.UID
	r.SourceServerID = a.p.Source.ServerID
	r.SourceNodeID = a.p.Source.NodeID
	a.report = r
	a.notifyCreated()
	a.pushActions()
	a.pushFinished()
	if a.report.Confirmed {
		// 库中战报已确认：补发完成无需再等确认回执，直接退出。
		a.finish(ctx)
	}
}

func (a *BattleActor) notifyCreated() {
	tickDurationMS := a.report.Result.GetTickDurationMS()
	if tickDurationMS <= 0 {
		tickDurationMS = engine.DefaultTickDurationMS
	}
	created := &pb.BattleCreatedNtf{BattleId: a.p.BattleID, Uid: a.p.UID, TickDurationMs: tickDurationMS}
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

// runBattle 用逻辑时间循环算完整场，保存初始快照后再运行以免取得最终血量。
func (a *BattleActor) runBattle(ctx tree.Context) {
	initialUnits := a.p.Battle.UnitResults()
	result, err := a.p.Battle.Run()
	if err != nil {
		tlog.Log.Error("[战斗/执行] 自动战斗结算失败, battleID=%v uid=%v err=%v", a.p.BattleID, a.p.UID, err)
		a.finish(ctx)
		return
	}
	a.logResult(result)

	now := time.Now().UTC()
	report := Report{BattleID: result.BattleID, UID: a.p.UID, SourceServerID: a.p.Source.ServerID, SourceNodeID: a.p.Source.NodeID, InitialUnits: initialUnits, Result: result, CreatedAt: now, UpdatedAt: now}
	if err := a.p.Reports.Save(context.Background(), report); err != nil {
		tlog.Log.Error("battle battle %s: save report: %v", a.p.BattleID, err)
		a.finish(ctx)
		return
	}
	a.report = report
	a.notifyCreated()
	a.pushActions()
	// 计算结束后只保留战报，释放引擎的单位、状态和时间轴。
	a.p.Battle = nil
	a.pushFinished()
}

func (a *BattleActor) logEvent(event *pb.BattleEvent) {
	if tlog.Log == nil {
		return
	}
	tlog.Log.Info(
		"[战斗/过程] battleID=%v seq=%v action=%v tick=%v type=%v actor=%v target=%v skill=%v status=%v amount=%v hp=%v->%v effect=%v statusType=%v critical=%v basic=%v reason=%v outcome=%v",
		a.p.BattleID, event.GetSequence(), event.GetAction(), event.GetTick(), event.GetType(),
		event.GetActorId(), event.GetTargetId(), event.GetSkillId(), event.GetStatusId(),
		event.GetAmount(), event.GetHpBefore(), event.GetHpAfter(), event.GetEffectType(),
		event.GetStatusType(), event.GetCritical(), event.GetBasicAttack(), event.GetReason(), event.GetOutcome(),
	)
	if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetHpBefore() > 0 && event.GetHpAfter() == 0 {
		tlog.Log.Info("[战斗/死亡] battleID=%v unit=%v killer=%v skill=%v action=%v tick=%v", a.p.BattleID, event.GetTargetId(), event.GetActorId(), event.GetSkillId(), event.GetAction(), event.GetTick())
	}
}

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
	tlog.Log.Info("[战斗/结算] 自动战斗完成, battleID=%v uid=%v outcome=%v tick=%v events=%v checksum=%v", result.BattleID, a.p.UID, result.Outcome, result.Tick, len(result.Events), result.Checksum)
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
	ntf := &pb.BattleFinishedNtf{BattleId: a.report.BattleID, Uid: a.report.UID, BattleType: result.BattleType, Outcome: result.Outcome, Units: result.Units, Checksum: result.Checksum}
	if err := a.p.Notifier.Notify(a.pushSource(), ntf); err != nil {
		tlog.Log.Warn("battle battle %s: notify Finished: %v", a.p.BattleID, err)
	}
	a.attempts++
	if !a.report.Confirmed {
		a.scheduleRetry()
	}
}

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

func (a *BattleActor) managerPID() tree.PID { return a.p.ManagerPID }

func (a *BattleActor) stopTimer() {
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

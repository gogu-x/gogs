package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// battleTickTimer 是本 actor 自己那台时间轮上的节拍任务类型（每个 wheel 内唯一即可）。
	battleTickTimer = timer.TimerType(1)
	// timeWheelChanLen 是时间轮到期队列的缓冲长度。参考 game/play 的取值。
	timeWheelChanLen = 16
	// battleMailboxSize 是 actor 邮箱容量。时间轮回调与普通用户消息共用这个邮箱，
	// 而 BattleResultConfirmedNtf 是从共享的 Nats actor goroutine 投递进来的
	// （投递是阻塞的），邮箱被填满会连带卡住整个进程的 NATS 路由，所以留足余量。
	battleMailboxSize = 256
)

// BattleActor 是一场战斗的独立 battle。整场战斗的"创建回执→按 tick 流式推送→
// 结算→落库→结束推送→确认→退出"全部在它自己的 goroutine 内串行完成。
// 注册名 = def.BattleActorName(battleID)，跨进程可被 NATS 按 TaggerName
// 直达（如 BattleResultConfirmedNtf 确认）。
type BattleActor struct {
	p          Params
	report     Report // 待推送/已推送的战报载体（流式期间为空，结算后才填充）
	attempts   int
	timer      *timer.TimeWheel
	released   bool
	system     *tree.Tree
	self       tree.PID
	battle     *engine.Battle // ModeRun 下的引擎实例，按 tick 增量推进
	paceAnchor time.Time      // 节拍锚点：tick N 的推送目标时刻 = paceAnchor + N*tickDuration
}

func NewBattleActor(p Params) *BattleActor {
	return &BattleActor{p: p}
}

func (a *BattleActor) Name() string     { return def.BattleActorName(a.p.BattleID) }
func (a *BattleActor) MailboxSize() int { return battleMailboxSize }

func (a *BattleActor) OnInit(ctx tree.Context) {
	a.system = ctx.System()
	a.self = ctx.Self()
	// Register 必须在 After 之前，否则 handler() 会 panic；而 OnInit 走 safeCall，
	// panic 只会被 recover 掉，表现为 actor 半初始化后继续跑，极难排查。
	a.timer = timer.NewTimeWheel(timeWheelChanLen, ctx.Self(), ctx.System())
	a.timer.Register(battleTickTimer, func(interface{}) {
		// 时间轮回调不发 ctx（SendCallback 会丢掉它），转投自己的邮箱，
		// 这样 HandleMessage 里拿到的是新鲜可用的 ctx。
		a.system.Send(a.self, &battleTick{})
	})
	a.begin(ctx)
}

func (a *BattleActor) OnStop(_ tree.Context) { a.stopTimer() }

func (a *BattleActor) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
	case *pb.BattleResultConfirmedNtf:
		a.confirm(ctx, m)
	case *battleTick:
		a.onTick(ctx)
	case *retryTick:
		a.retry(ctx)
	case *RetireBattle:
		a.finish(ctx)
	default:
		ctx.Response(nil, fmt.Errorf("battle battle %s: unsupported message %T", a.p.BattleID, msg))
	}
}

// begin 在 OnInit 中把一场战斗推进到"开始按 tick 推送"或"已推送完成、等待确认"。
//
// ModeRun 顺序保证：Created → 按 tick 分批的 Action 流 → 结算并 Save → Finished。
// ModeRedeliver 顺序保证：Created → 全部 Action → Finished（战报已在库中，直接快推）。
func (a *BattleActor) begin(ctx tree.Context) {
	if a.p.Mode == ModeRun && a.p.Battle == nil {
		compiler := a.p.Compiler
		if compiler == nil {
			compiler = configcompiler.NewDefault()
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
	a.notifyCreated()

	if a.p.Mode == ModeRun {
		a.battle = a.p.Battle
		if tlog.Log != nil {
			tlog.Log.Info("[战斗/执行] 开始自动战斗, battleID=%v uid=%v seed=%v", a.p.BattleID, a.p.UID, a.p.Seed)
		}
		// 节拍锚点：tick N 的推送目标时刻 = paceAnchor + N*tickDuration。
		// 把首个 tick 反推到"现在"，让首个动作立刻开始演出；后续 tick 的相对
		// 间距不变，因此整条时间轴仍然与客户端的时间轴对齐。
		duration := a.tickDuration()
		a.paceAnchor = time.Now()
		if duration > 0 {
			if first, ok := a.battle.NextTick(); ok && first > 0 {
				a.paceAnchor = a.paceAnchor.Add(-time.Duration(first) * duration)
			}
		}
		a.onTick(ctx)
		return
	}

	// ModeRedeliver：复用库中已有战报，但把投递回执目标覆盖为当前请求方。
	r := a.p.Report
	r.UID = a.p.UID
	r.SourceServerID = a.p.Source.ServerID
	r.SourceNodeID = a.p.Source.NodeID
	a.report = r
	a.pushActions()
	a.pushFinished()
	if a.report.Confirmed {
		// 库中战报已确认：补发完成无需再等确认回执，直接退出。
		a.finish(ctx)
	}
}

func (a *BattleActor) notifyCreated() {
	tickDurationMS := engine.DefaultTickDurationMS
	if a.p.Battle != nil {
		tickDurationMS = a.p.Battle.TickDurationMS()
	} else if a.p.Report.Result.TickDurationMS > 0 {
		tickDurationMS = a.p.Report.Result.TickDurationMS
	}
	created := &pb.BattleCreatedNtf{BattleId: a.p.BattleID, Uid: a.p.UID, TickDurationMs: tickDurationMS}
	// 参战名单在创建时即已确定，随 Created 一起下发，客户端进战斗页就能把双方单位摆好。
	if a.p.Battle != nil {
		created.Units = a.p.Battle.UnitResults()
	} else if a.p.Report.Result.Units != nil {
		created.Units = a.p.Report.Result.Units
	}
	if err := a.p.Notifier.Notify(a.p.Source, created); err != nil {
		tlog.Log.Warn("battle battle %s: notify Created: %v", a.p.BattleID, err)
	}
}

// pushActions 把库中已有战报的全部事件一次性推完（仅重投路径使用）。
func (a *BattleActor) pushActions() {
	a.pushEvents(a.report.BattleID, a.report.UID, a.report.Result.Events)
}

// pushEvents 逐条推送一批战斗事件。调用方一次只传一个 tick 的事件，避免长时间
// 占住 actor goroutine —— Notify 是带超时的同步往返，不是 fire-and-forget。
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

// tickDuration 返回一个 tick 对应的真实墙钟时长。
// PaceTickDuration > 0 直接覆盖；< 0 表示不节流（测试用），返回 0。
func (a *BattleActor) tickDuration() time.Duration {
	if a.p.PaceTickDuration > 0 {
		return a.p.PaceTickDuration
	}
	if a.p.PaceTickDuration < 0 {
		return 0
	}
	if a.battle != nil {
		return time.Duration(a.battle.TickDurationMS()) * time.Millisecond
	}
	return time.Duration(engine.DefaultTickDurationMS) * time.Millisecond
}

// onTick 推进一个节拍：推送当前 tick 的事件，再按节拍锚点排下一个节拍。
// 不节流时（tickDuration <= 0）在一个循环里推到底，供测试与重投路径使用。
func (a *BattleActor) onTick(ctx tree.Context) {
	if a.battle == nil {
		return
	}
	duration := a.tickDuration()
	for {
		if _, ok := a.battle.NextTick(); !ok {
			a.finalizeBattle(ctx)
			return
		}
		a.pushEvents(a.p.BattleID, a.p.UID, a.battle.Advance())
		if duration <= 0 {
			continue
		}
		next, ok := a.battle.NextTick()
		if !ok {
			// 刚推完最后一个 tick，立刻收尾，不必再等一个节拍。
			a.finalizeBattle(ctx)
			return
		}
		// 用绝对锚点算延迟：时间轮会把延迟向上取整到 10ms，
		// 按增量排期会逐跳累积漂移。
		target := a.paceAnchor.Add(time.Duration(next) * duration)
		delay := time.Until(target)
		if delay < 0 {
			delay = 0
		}
		a.timer.After(battleTickTimer, delay, nil)
		return
	}
}

// finalizeBattle 结束模拟：推送收尾事件 → 落库 → 推送 Finished。
// 顺序不可颠倒：report.go 明确要求 Finished 不得早于战报落库。
// Checksum 是全量事件的哈希，因此 Finished 必须在整场结束后才发。
func (a *BattleActor) finalizeBattle(ctx tree.Context) {
	result, err := a.battle.Finalize()
	if err != nil {
		tlog.Log.Error("[战斗/执行] 自动战斗结算失败, battleID=%v uid=%v err=%v", a.p.BattleID, a.p.UID, err)
		a.finish(ctx)
		return
	}
	a.pushEvents(a.p.BattleID, a.p.UID, a.battle.Flush())
	a.logResult(result)

	now := time.Now().UTC()
	report := Report{BattleID: result.BattleID, UID: a.p.UID, SourceServerID: a.p.Source.ServerID, SourceNodeID: a.p.Source.NodeID, Result: result, CreatedAt: now, UpdatedAt: now}
	if err := a.p.Reports.Save(context.Background(), report); err != nil {
		tlog.Log.Error("battle battle %s: save report: %v", a.p.BattleID, err)
		a.finish(ctx)
		return
	}
	a.report = report
	a.pushFinished()
}

func (a *BattleActor) logEvent(event *pb.BattleEvent) {
	if tlog.Log == nil {
		return
	}
	tlog.Log.Info("[战斗/过程] battleID=%v seq=%v action=%v tick=%v type=%v actor=%v target=%v skill=%v status=%v amount=%v hp=%v->%v detail=%v", a.p.BattleID, event.GetSequence(), event.GetAction(), event.GetTick(), event.GetType(), event.GetActorId(), event.GetTargetId(), event.GetSkillId(), event.GetStatusId(), event.GetAmount(), event.GetHpBefore(), event.GetHpAfter(), event.GetDetail())
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

// pushSource 返回推送目标。
// 流式推送发生在战报落库之前（那时 a.report 还是零值），若直接读它会得到
// Source{0,0}，事件会被投递到不存在的 0 号节点而静默丢弃。此时回落到创建请求方 ——
// 两条路径的目标本来就相同：begin 里对 report 的赋值用的就是 a.p.Source。
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

func (a *BattleActor) scheduleRetry() {
	a.stopTimer()
	//a.timer = a.timer.After(a.p.RetryDelay, func() {
	//	a.system.Send(a.self, &retryTick{})
	//})
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
	if a.system != nil && a.managerPID().ID != 0 {
		a.system.Send(a.managerPID(), &ActorStopped{BattleID: a.p.BattleID, PID: a.self})
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

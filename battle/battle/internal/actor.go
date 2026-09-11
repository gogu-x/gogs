package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/def"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

// BattleActor 是一场战斗的独立 battle。整场战斗的"创建回执→结算→落库→
// 事件/结束推送→确认/重试→退出"全部在它自己的 goroutine 内串行完成。
// 注册名 = def.BattleActorName(battleID)，跨进程可被 NATS 按 TaggerName
// 直达（如 BattleResultConfirmedNtf 确认）。
type BattleActor struct {
	p        Params
	report   Report // 待推送/已推送的战报载体
	attempts int
	timer    *timer.TimeWheel
	released bool
	system   *tree.Tree
	self     tree.PID
}

func NewBattleActor(p Params) *BattleActor {
	return &BattleActor{p: p}
}

func (a *BattleActor) Name() string     { return def.BattleActorName(a.p.BattleID) }
func (a *BattleActor) MailboxSize() int { return 12 }

func (a *BattleActor) OnInit(ctx tree.Context) {
	a.system = ctx.System()
	a.self = ctx.Self()
	a.begin(ctx)
}

func (a *BattleActor) OnStop(_ tree.Context) { a.stopTimer() }

func (a *BattleActor) HandleMessage(ctx tree.Context, msg interface{}) {
	switch m := msg.(type) {
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

// begin 在 OnInit 中推进一场战斗到"已推送完成、等待确认"。
// 顺序保证：Created → (run)结算并 Save → Action 流 → Finished。
func (a *BattleActor) begin(ctx tree.Context) {
	if a.p.Mode == ModeRun && a.p.Battle == nil {
		battle, err := newBattleFromRequest(a.p.BattleID, a.p.Seed, a.p.Request)
		if err != nil {
			tlog.Log.Error("[战斗/创建] 读取配置并创建引擎失败, battleID=%v uid=%v err=%v", a.p.BattleID, a.p.UID, err)
			a.finish(ctx)
			return
		}
		a.p.Battle = battle
	}
	a.notifyCreated()
	if a.p.Mode == ModeRun {
		if tlog.Log != nil {
			tlog.Log.Info("[战斗/执行] 开始自动战斗, battleID=%v uid=%v seed=%v", a.p.BattleID, a.p.UID, a.p.Seed)
		}
		result, err := a.p.Battle.Run()
		if err != nil {
			tlog.Log.Error("[战斗/执行] 自动战斗失败, battleID=%v uid=%v err=%v", a.p.BattleID, a.p.UID, err)
			a.finish(ctx)
			return
		}
		if tlog.Log != nil {
			for _, event := range result.Events {
				if event == nil {
					continue
				}
				tlog.Log.Info("[战斗/过程] battleID=%v seq=%v action=%v tick=%v type=%v actor=%v target=%v skill=%v status=%v amount=%v hp=%v->%v detail=%v", result.BattleID, event.GetSequence(), event.GetAction(), event.GetTick(), event.GetType(), event.GetActorId(), event.GetTargetId(), event.GetSkillId(), event.GetStatusId(), event.GetAmount(), event.GetHpBefore(), event.GetHpAfter(), event.GetDetail())
				if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetHpBefore() > 0 && event.GetHpAfter() == 0 {
					tlog.Log.Info("[战斗/死亡] battleID=%v unit=%v killer=%v skill=%v action=%v tick=%v", result.BattleID, event.GetTargetId(), event.GetActorId(), event.GetSkillId(), event.GetAction(), event.GetTick())
				}
			}
			for _, unit := range result.Units {
				if unit == nil {
					continue
				}
				tlog.Log.Info("[战斗/单位] battleID=%v unit=%v team=%v hp=%v/%v alive=%v", result.BattleID, unit.GetInstanceId(), unit.GetTeam(), unit.GetHp(), unit.GetMaxHp(), unit.GetAlive())
			}
			tlog.Log.Info("[战斗/结算] 自动战斗完成, battleID=%v uid=%v outcome=%v tick=%v events=%v checksum=%v", result.BattleID, a.p.UID, result.Outcome, result.Tick, len(result.Events), result.Checksum)
		}
		now := time.Now().UTC()
		report := Report{BattleID: result.BattleID, UID: a.p.UID, SourceServerID: a.p.Source.ServerID, SourceNodeID: a.p.Source.NodeID, Result: result, CreatedAt: now, UpdatedAt: now}
		if err := a.p.Reports.Save(context.Background(), report); err != nil {
			tlog.Log.Error("battle battle %s: save report: %v", a.p.BattleID, err)
			a.finish(ctx)
			return
		}
		a.report = report
	} else {
		// 复用库中已有战报，但把投递回执目标覆盖为当前请求方。
		r := a.p.Report
		r.UID = a.p.UID
		r.SourceServerID = a.p.Source.ServerID
		r.SourceNodeID = a.p.Source.NodeID
		a.report = r
	}
	a.pushActions()
	a.pushFinished()
	if a.p.Mode == ModeRedeliver && a.report.Confirmed {
		// 库中战报已确认：补发完成无需再等确认回执，直接退出。
		a.finish(ctx)
	}
}

func (a *BattleActor) notifyCreated() {
	created := &pb.BattleCreatedNtf{BattleId: a.p.BattleID, Uid: a.p.UID}
	if err := a.p.Notifier.Notify(a.p.Source, created); err != nil {
		tlog.Log.Warn("battle battle %s: notify Created: %v", a.p.BattleID, err)
	}
}

func (a *BattleActor) pushActions() {
	source := a.pushSource()
	for _, event := range a.report.Result.Events {
		ntf := &pb.BattleActionNtf{BattleId: a.report.BattleID, Uid: a.report.UID, Event: event}
		if err := a.p.Notifier.Notify(source, ntf); err != nil {
			tlog.Log.Warn("battle battle %s: notify Action %d: %v", a.p.BattleID, event.GetSequence(), err)
		}
	}
}

func (a *BattleActor) pushSource() Source {
	return Source{ServerID: a.report.SourceServerID, NodeID: a.report.SourceNodeID}
}

// pushFinished 推送 Finished 并（未确认时）安排重试。
func (a *BattleActor) pushFinished() {
	result := a.report.Result
	ntf := &pb.BattleFinishedNtf{BattleId: a.report.BattleID, Uid: a.report.UID, Outcome: result.Outcome, Units: result.Units, Checksum: result.Checksum}
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

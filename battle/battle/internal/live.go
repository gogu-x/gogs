package internal

import (
	"context"
	"time"

	"github.com/gogu-x/gogs/battle/robot"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// liveBattle 保存单场即时战斗的时钟、玩家控制信息和 AI 调度入口，由 BattleActor 串行维护。
type liveBattle struct {
	start       time.Time // 开战时刻的起点，战斗与机器人使用同一时间基准。
	robotPID    tree.PID  // 本场防守单位共用的机器人 Actor，零值表示没有运行中的机器人。
	statusDueMS int64     // 最近安排唤醒的毫秒时间；-1 表示尚未安排或已开始处理该唤醒。
}

// beginLive 在引擎创建后记录初始快照，分配单位控制权，再通知 Game 开战。
func (a *BattleActor) beginLive() {
	b := a.p.Battle
	now := time.Now()
	live := &liveBattle{start: now, statusDueMS: -1}
	a.live = live
	units := b.UnitResults()
	a.report = Report{BattleID: a.p.BattleID, UID: a.p.UID, SourceServerID: a.p.Source.ServerID, SourceNodeID: a.p.Source.NodeID, InitialUnits: units, CreatedAt: now.UTC()}
	// 进攻方由玩家客户端出手；防守单位分别登记策略和首击延迟，登记完成后统一启动机器人。
	var ai *robot.RobotActor
	for i, control := range b.Controls() {
		if units[i].GetTeam() == pb.BattleTeam_BATTLE_TEAM_ATTACKER {
			continue
		}
		if ai == nil {
			ai = robot.New(a.p.BattleID, a.self, now)
		}
		id := control.GetUnitId()
		ai.AddUnit(control, b.UnitPosition(id),
			b.RobotFirstDelay(id), robot.ConfiguredRotation(b.RobotSequence(id)), b.RobotRules(id))
	}
	if ai != nil {
		live.robotPID = tree.SpawnOne(ai)
	}
	for _, m := range a.members {
		a.sendSnapshot(m, false)
	}
	a.pushLiveEvents(b.Flush())
	a.scheduleStatus()
}

// stopRobots 在战斗结算或 Actor 停止时回收本场机器人，清除 PID 以便重复调用。
func (a *BattleActor) stopRobots() {
	if a.live == nil {
		return
	}
	if pid := a.live.robotPID; pid.ID != 0 {
		tree.Stop(pid)
		a.live.robotPID = tree.PID{}
	}
}

// advanceLive 将引擎推进到服务端当前战斗时间，先结算已到期的状态效果。
// 返回 false 时本场已结束或没有可用引擎，调用方应停止处理当前消息。
func (a *BattleActor) advanceLive(ctx tree.Context) bool {
	if a.live == nil || a.p.Battle == nil {
		return false
	}
	b := a.p.Battle
	now := time.Since(a.live.start).Milliseconds()
	a.pushLiveEvents(b.AdvanceClock(now))
	if b.InputDone() {
		a.completeLive(ctx)
		return false
	}
	return true
}

// liveStatus 消费唤醒消息，推进所有已到期状态，并按推进后的队首安排下次唤醒。
func (a *BattleActor) liveStatus(ctx tree.Context) {
	if a.live == nil {
		return
	}
	a.live.statusDueMS = -1
	if a.advanceLive(ctx) {
		a.scheduleStatus()
	}
}

// liveInput 接收玩家或机器人提交的动作。先推进状态，再由引擎按来源阵营校验并结算，
// 使当前时刻已经到期的控制效果或致死伤害能影响这次出手。
func (a *BattleActor) liveInput(ctx tree.Context, req *pb.BattleInputReq, fromRobot bool) {
	if req == nil || req.GetBattleId() != a.p.BattleID {
		return
	}
	var owner *Member
	if !fromRobot {
		owner = a.member(req.GetUID())
		if owner == nil {
			return
		}
		if !owner.owns(req.GetUnitId()) {
			a.p.Notifier.Notify(owner.Source, &pb.BattleInputAck{Uid: owner.UID, BattleId: a.p.BattleID, UnitId: req.GetUnitId(), Sequence: req.GetSequence(), Code: common.ErrCode_INVALID_BATTLE_REQUEST})
			return
		}
	}
	if !a.advanceLive(ctx) {
		return
	}
	team := pb.BattleTeam_BATTLE_TEAM_ATTACKER
	if fromRobot {
		team = pb.BattleTeam_BATTLE_TEAM_DEFENDER
	}
	b := a.p.Battle
	ack := b.ApplyInput(req, team)
	a.pushLiveEvents(b.Flush())
	// 过程事件统一经 Game 推送；动作回执单独返回给出手方，用于安排该单位下次行动。
	if fromRobot {
		if pid := a.live.robotPID; pid.ID != 0 {
			ctx.Send(pid, ack)
		}
	} else {

		if err := a.p.Notifier.Notify(owner.Source, ack); err != nil {
			tlog.Log.Warn("[战斗/liveInput] 动作回执投递失败, pid=%v err=%v", a.p.UID, err)
		}
	}
	if b.InputDone() {
		a.completeLive(ctx)
		return
	}
	a.scheduleStatus()
}

// completeLive 停止 AI 调度，汇总并保存战报。保存成功后推送终局事件和结束通知，
// 随后由已有的确认与重试流程处理结果投递。
func (a *BattleActor) completeLive(ctx tree.Context) {
	a.stopRobots()
	b := a.p.Battle
	result, err := b.Finalize()
	if err == nil {
		a.logResult(result)
		a.report.Members = a.members
		a.report.Result = result
		a.report.UpdatedAt = time.Now().UTC()
		err = a.p.Reports.Save(context.Background(), a.report)
	}
	if err != nil {
		tlog.Log.Error("[战斗/completeLive] 保存战报失败, pid=%v err=%v", a.p.UID, err)
		a.finish(ctx)
		return
	}
	a.pushLiveEvents(b.Flush())
	a.p.Battle = nil
	a.pushFinished()
}

// pushLiveEvents 同步死亡单位的调度状态，再转发战斗事件。
func (a *BattleActor) pushLiveEvents(events []*pb.BattleEvent) {
	if pid := a.live.robotPID; pid.ID != 0 {
		for _, event := range events {
			if event.GetType() == pb.BattleEventType_BATTLE_EVENT_TYPE_DAMAGE && event.GetHpAfter() == 0 {
				tree.Send(pid, &robot.UnitRemoved{UnitID: event.GetTargetId()})
			}
		}
	}
	for _, event := range events {
		a.logEvent(event)
	}
	for _, m := range a.members {
		a.pushMemberEvents(m, events)
	}
	a.offerTowerDrop(events)
}

// observeRobots 为同批到期的 AI 单位提供当前血量和各自的可用技能。
// RobotActor 根据快照选招和目标；动作返回 BattleActor 后仍按当时的状态校验。
func (a *BattleActor) observeRobots(ctx tree.Context, query *robot.Observe) {
	if !a.advanceLive(ctx) {
		return
	}
	b := a.p.Battle
	view := &robot.DecisionView{Units: b.UnitResults(), Requests: query.Requests, Skills: make(map[string]map[string]pb.TargetRule)}
	for _, req := range query.Requests {
		id := req.GetUnitId()
		view.Skills[id] = b.AvailableSkills(id)
	}
	ctx.Send(a.live.robotPID, view)
}

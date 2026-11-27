package engine

import (
	"container/heap"
	"github.com/gogu-x/gogs/battle/robot/strategy"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

type inputState struct {
	actor *unit
	ready int64
	last  *pb.BattleInputAck
}

// Controls 返回客户端和机器人可使用的单位动作资料。
func (b *Battle) Controls() []*pb.BattleControl {
	var controls []*pb.BattleControl
	for _, actor := range b.units {
		control := b.control(actor)
		controls = append(controls, control)
	}
	return controls
}

// control 生成指定单位当前出手时间、序号及动作的快照。
func (b *Battle) control(actor *unit) *pb.BattleControl {
	config := actor.Config
	control := &pb.BattleControl{
		CanGuard: config.GuardEnabled, GuardReadyMs: actor.guardReady,
		UnitId: config.InstanceID, BasicSkill: config.BasicSkill.ID,
		Attack: b.unitAttack(actor), Defense: b.unitDefense(actor),
		Speed: b.unitSpeed(actor), ActionIntervalMs: b.attackInterval(actor),
	}
	if state := b.inputs[config.InstanceID]; state != nil {
		control.NextReadyMs = state.ready
		control.LastInputSequence = state.last.GetSequence()
	}
	for _, skill := range config.ActiveSkills {
		control.ActiveSkills = append(control.ActiveSkills, skill.ID)
	}
	return control
}

// AdvanceClock 先推进到期状态与已锁定的重击，在截止时刻停止接受动作。
func (b *Battle) AdvanceClock(now int64) []*pb.BattleEvent {
	start := b.emitted
	for {
		next, ok := b.NextDueMS()
		if !ok || next > now {
			break
		}
		if next == b.setup.Rules.MaxDurationMS {
			b.elapsedMS = next
			b.done = true
			b.finalOutcome = pb.BattleOutcome_BATTLE_OUTCOME_DRAW
			break
		}
		item := heap.Pop(&b.timeline).(*timelineItem)
		b.elapsedMS = next
		if item.Phase == phaseStrikeImpact {
			b.landStrike(item.Strike)
		} else {
			b.handleStatusTask(item)
		}
		b.checkInputEnd()
		b.emitted = len(b.events)
	}
	if !b.done {
		b.elapsedMS = max(b.elapsedMS, now)
	}
	return b.events[start:b.emitted]
}

// InputDone 表示输入驱动战斗已决出结果或达到时长上限。
func (b *Battle) InputDone() bool { return b.done }

// ApplyInput 校验动作所属阵营、序号、冷却和目标并即时结算。
func (b *Battle) ApplyInput(req *pb.BattleInputReq, team pb.BattleTeam) *pb.BattleInputAck {
	ack := &pb.BattleInputAck{Uid: req.GetUID(), Code: pb_common.ErrCode_INVALID_BATTLE_REQUEST, BattleId: b.ID(), UnitId: req.GetUnitId(), Sequence: req.GetSequence(), CurrentMs: b.elapsedMS}
	state := b.inputs[req.GetUnitId()]
	if state == nil || req.GetBattleId() != b.ID() || state.actor.Config.Team != team || req.GetSequence() == 0 {
		return ack
	}
	if last := state.last; last != nil {
		if req.GetSequence() == last.GetSequence() {
			return last
		}
		if req.GetSequence() < last.GetSequence() {
			return ack
		}
	}
	state.last = ack
	ack.ReadyMs = state.ready
	if b.done || !state.actor.alive() {
		return ack
	}
	if req.GetSkillId() == guardSkillID {
		b.startGuard(state, req, ack)
		return ack
	}
	if b.elapsedMS < state.ready || state.actor.windup != nil {
		return ack
	}
	actor := state.actor
	skill := actor.Config.BasicSkill
	if req.GetSkillId() != skill.ID {
		found := false
		for _, candidate := range actor.Config.ActiveSkills {
			if candidate.ID == req.GetSkillId() && b.canCastActive(actor, candidate.ID) {
				skill = candidate
				found = true
				break
			}
		}
		if !found {
			return ack
		}
	}
	targets := b.targets(actor, skill.TargetRule)
	if id := req.GetTargetId(); id != "" {
		if skill.TargetRule == pb.TargetRule_TARGET_RULE_ENEMY_SINGLE {
			targets = nil
			for _, target := range b.units {
				if target.Config.InstanceID == id && target.Config.Team != team && target.alive() {
					targets = []*unit{target}
					break
				}
			}
		} else if len(targets) != 1 || targets[0].Config.InstanceID != id {
			return ack
		}
	}
	if len(targets) == 0 {
		return ack
	}
	b.beginAction()
	action := &battleAction{Number: b.action, Actor: actor, Skill: skill, Targets: targets, TargetIDs: targetIDs(targets), Basic: skill.ID == actor.Config.BasicSkill.ID}
	state.ready = b.elapsedMS + b.attackInterval(actor)
	ack.Code = pb_common.ErrCode_OK
	ack.Action = b.action
	ack.ReadyMs = state.ready
	b.tickCooldowns(actor)
	b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED)
	b.tickPeriodic(actor)
	if !actor.alive() || b.hasStatus(actor, pb.StatusType_STATUS_TYPE_STUN) {
		b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_SKIPPED)
	} else {
		if !action.Basic {
			actor.Cooldowns[skill.ID] = skill.Cooldown
		}
		b.emitSkillUsed(action)
		if skill.WindupMS > 0 {
			b.startWindup(state, action)
			ack.ReadyMs = state.ready
			return ack
		}
		b.impactAction(action)
	}
	b.endAction(action)
	ack.ReadyMs = state.ready
	b.checkInputEnd()
	return ack
}

// checkInputEnd 检查即时战斗是否已满足结束条件。
func (b *Battle) checkInputEnd() {
	if outcome, done := b.outcome(); done {
		b.finalOutcome = outcome
		b.done = true
		return
	}
}

// RobotSequence 返回该单位开战时确定的出招配置。
func (b *Battle) RobotSequence(unitID string) []string {
	if state := b.inputs[unitID]; state != nil {
		return state.actor.Config.RobotSequence
	}
	return nil
}

// UnitPosition 返回配置站位，用于同一时刻机器人决策排序。
func (b *Battle) UnitPosition(unitID string) int32 { return b.inputs[unitID].actor.Config.Position }

// RobotFirstDelay 返回机器人策略配置的开场等待毫秒数。
func (b *Battle) RobotFirstDelay(unitID string) int64 {
	return b.inputs[unitID].actor.Config.RobotFirstDelay
}

// canCastActive 按下一次行动开始时的冷却和当前沉默状态判断技能资格。
func (b *Battle) canCastActive(actor *unit, id string) bool {
	return actor.Cooldowns[id] <= 1 && !b.hasStatus(actor, pb.StatusType_STATUS_TYPE_SILENCE)
}

// RobotRules 返回开战后只读的施法策略。
func (b *Battle) RobotRules(id string) []strategy.Rule { return b.inputs[id].actor.Config.CastRules }

// AvailableSkills 为机器人创建独立技能快照，实际施法仍由 ApplyInput 再次校验。
func (b *Battle) AvailableSkills(id string) map[string]pb.TargetRule {
	state := b.inputs[id]
	if state == nil || !state.actor.alive() {
		return nil
	}
	actor := state.actor
	basic := actor.Config.BasicSkill
	skills := map[string]pb.TargetRule{basic.ID: basic.TargetRule}
	for _, skill := range actor.Config.ActiveSkills {
		if b.canCastActive(actor, skill.ID) {
			skills[skill.ID] = skill.TargetRule
		}
	}
	return skills
}

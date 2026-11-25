package engine

import (
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
		config := actor.Config
		control := &pb.BattleControl{UnitId: config.InstanceID, BasicSkill: config.BasicSkill.ID}
		if state := b.inputs[config.InstanceID]; state != nil {
			control.NextReadyTick = state.ready
			control.LastInputSequence = state.last.GetSequence()
		}
		for _, skill := range config.ActiveSkills {
			control.ActiveSkills = append(control.ActiveSkills, skill.ID)
		}
		controls = append(controls, control)
	}
	return controls
}

// AdvanceClock 仅处理已到期的限时状态任务。
func (b *Battle) AdvanceClock(now int64) []*pb.BattleEvent {
	var events []*pb.BattleEvent
	for {
		next, ok := b.NextTick()
		if !ok || next > now {
			break
		}
		events = append(events, b.Advance()...)
	}
	if !b.done {
		b.tick = max(b.tick, now)
	}
	return events
}

// InputDone 表示输入驱动战斗已决出结果或耗尽行动次数。
func (b *Battle) InputDone() bool { return b.done }

// ApplyInput 校验动作所属阵营、序号、冷却和目标并即时结算。
func (b *Battle) ApplyInput(req *pb.BattleInputReq, team pb.BattleTeam) *pb.BattleInputAck {
	ack := &pb.BattleInputAck{Uid: req.GetUID(), Code: pb_common.ErrCode_INVALID_BATTLE_REQUEST, BattleId: b.ID(), UnitId: req.GetUnitId(), Sequence: req.GetSequence(), CurrentTick: b.tick}
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
	ack.ReadyTick = state.ready
	if b.done || !state.actor.alive() {
		return ack
	}
	if b.tick < state.ready || b.startedActions >= b.setup.Rules.MaxActions {
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
	b.nextAction++
	b.startedActions++
	b.action = b.nextAction
	b.activeActions++
	action := &scheduledAction{ID: b.action, Actor: actor, Skill: skill, Targets: targets, TargetIDs: targetIDs(targets), StartTick: b.tick, ImpactTick: b.tick, EndTick: b.tick, Basic: skill.ID == actor.Config.BasicSkill.ID}
	state.ready = b.tick + b.attackInterval(actor)
	ack.Code = pb_common.ErrCode_OK
	ack.Action = action.ID
	ack.ReadyTick = state.ready
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
		b.impactAction(action)
	}
	b.endAction(action)
	b.checkInputEnd()
	return ack
}

func (b *Battle) checkInputEnd() {
	if outcome, done := b.outcome(); done {
		b.finalOutcome = outcome
		b.terminal = true
		b.done = true
		return
	}
	if b.startedActions >= b.setup.Rules.MaxActions && b.activeActions == 0 {
		b.finalOutcome = pb.BattleOutcome_BATTLE_OUTCOME_DRAW
		b.done = true
	}
}

// RobotSequence 返回该单位开战时确定的出招配置。
func (b *Battle) RobotSequence(unitID string) []string {
	if state := b.inputs[unitID]; state != nil {
		return state.actor.Config.RobotSequence
	}
	return nil
}

// UnitPosition 返回配置站位，用于同帧机器人决策排序。
func (b *Battle) UnitPosition(unitID string) int32 { return b.inputs[unitID].actor.Config.Position }

// RobotFirstDelay 返回机器人策略配置的开场等待帧数。
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

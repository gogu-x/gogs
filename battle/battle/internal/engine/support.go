package engine

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

// isSupportSkill 仅将友方纯治疗、持续治疗及非负属性增益从攻击选招中分离。
// 伤害、敌方控制、减益和未知效果继续使用原有动作窗口。
func isSupportSkill(skill SkillConfig) bool {
	if skill.TargetRule != pb.TargetRule_TARGET_RULE_SELF &&
		skill.TargetRule != pb.TargetRule_TARGET_RULE_ALLY_LOWEST_HP {
		return false
	}
	if len(skill.Effects) == 0 || len(skill.Charges) != 0 {
		return false
	}
	for _, effect := range skill.Effects {
		switch effect.Kind {
		case pb.EffectType_EFFECT_TYPE_HEAL:
		case pb.EffectType_EFFECT_TYPE_APPLY_STATUS:
			if !isBeneficialStatus(effect.Status) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// isBeneficialStatus 防止无伤害的控制或负面属性技能被当作免费增益触发。
func isBeneficialStatus(status *StatusConfig) bool {
	if status == nil {
		return false
	}
	switch status.Kind {
	case pb.StatusType_STATUS_TYPE_HOT:
		return status.Potency >= 0
	case pb.StatusType_STATUS_TYPE_ATTRIBUTE_MODIFIER:
		modifier := status.Modifier
		return modifier.AttackFlat >= 0 && modifier.DefenseFlat >= 0 && modifier.SpeedFlat >= 0
	default:
		return false
	}
}

// triggerSupportSkills 冷却在正常行动开始时已推进一次；附加技能不重复推进。
// 每项至多触发一次，零冷却也不会递归排入同帧行动。
func (b *Battle) triggerSupportSkills(actor *unit) {
	if b.hasStatus(actor, pb.StatusType_STATUS_TYPE_SILENCE) {
		return
	}
	attackAction := b.action
	defer func() { b.action = attackAction }()
	for _, skill := range actor.Config.ActiveSkills {
		if actor.Cooldowns[skill.ID] == 0 && isSupportSkill(skill) {
			b.triggerSupportSkill(actor, skill)
		}
	}
}

// triggerSupportSkill 用独立 action 编号同步结算效果；不增加行动上限计数，
// 不安排前后摇、下一次出手，也不递减状态回合。事件仍保留完整关联窗口。
func (b *Battle) triggerSupportSkill(actor *unit, skill SkillConfig) {
	targets := b.targets(actor, skill.TargetRule)
	b.nextAction++
	b.action = b.nextAction
	tick := b.tick
	action := &scheduledAction{
		ID: b.action, Actor: actor, Skill: skill, Targets: targets, TargetIDs: targetIDs(targets),
		StartTick: tick, ImpactTick: tick, EndTick: tick,
	}
	b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_STARTED)
	b.emitSkillUsed(action)
	if skill.Cooldown > 0 {
		actor.Cooldowns[skill.ID] = skill.Cooldown
	}
	b.impactAction(action)
	b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_ACTION_ENDED)
}

// emitActionEvent 统一动作边界与技能事件的角色、生命值和目标关联字段。
func (b *Battle) emitActionEvent(action *scheduledAction, kind pb.BattleEventType) *pb.BattleEvent {
	actor, ids := action.Actor, action.TargetIDs
	return b.emit(
		kind, actor.Config.InstanceID, firstTarget(ids), action.Skill.ID, "",
		0, actor.HP, actor.HP,
		eventInfo{BasicAttack: kind == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED && action.Basic}, ids,
	)
}

// emitSkillUsed 对攻击和即时辅助效果都下发可直接消费的完整时间窗口。
func (b *Battle) emitSkillUsed(action *scheduledAction) {
	event := b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED)
	event.StartTick = action.StartTick
	event.ImpactTick = action.ImpactTick
	event.EndTick = action.EndTick
}

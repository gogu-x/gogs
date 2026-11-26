package engine

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

type battleAction struct {
	Actor     *unit
	Skill     SkillConfig
	Targets   []*unit
	TargetIDs []string
	Basic     bool
}

// emitActionEvent 统一动作边界与技能事件的角色、生命值和目标关联字段。
func (b *Battle) emitActionEvent(action *battleAction, kind pb.BattleEventType) *pb.BattleEvent {
	actor, ids := action.Actor, action.TargetIDs
	return b.emit(
		kind, actor.Config.InstanceID, firstTarget(ids), action.Skill.ID, "",
		0, actor.HP, actor.HP,
		eventInfo{BasicAttack: kind == pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED && action.Basic}, ids,
	)
}

// emitSkillUsed 对攻击和即时辅助效果都下发可直接消费的完整时间窗口。
func (b *Battle) emitSkillUsed(action *battleAction) {
	event := b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED)
	event.StartMs = b.elapsedMS
	event.ImpactMs = b.elapsedMS
	event.EndMs = b.elapsedMS
}

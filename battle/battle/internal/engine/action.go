package engine

import pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"

type battleAction struct {
	Actor     *unit
	Skill     SkillConfig
	Targets   []*unit
	TargetIDs []string
	Basic     bool
	Number    uint64 // 举刀与命中共用的原动作编号。
}

// beginAction 为新的出招或架势分配场内单调递增的动作编号。
func (b *Battle) beginAction() {
	b.startedActions++
	b.action = uint64(b.startedActions)
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

// emitSkillUsed 下发出手、命中和收招的战斗时间，供客户端还原举刀动作。
func (b *Battle) emitSkillUsed(action *battleAction) {
	event := b.emitActionEvent(action, pb.BattleEventType_BATTLE_EVENT_TYPE_SKILL_USED)
	event.StartMs = b.elapsedMS
	event.ImpactMs = b.elapsedMS + action.Skill.WindupMS
	event.EndMs = event.GetImpactMs()
}

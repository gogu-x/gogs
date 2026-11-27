package engine

import (
	"math"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
)

const (
	// guardSkillID 是玩家架势输入使用的保留动作编号。
	guardSkillID = "guard"
	// guardWindowMS 是服务端接受架势后可格挡的毫秒数。
	guardWindowMS int64 = 800
	// guardCooldownMS 是两次架势之间的最短毫秒数。
	guardCooldownMS int64 = 3000
)

// startGuard 独立校验架势，推迟下一次普通行动，成功后发布防守状态。
func (b *Battle) startGuard(state *inputState, req *pb.BattleInputReq, ack *pb.BattleInputAck) {
	actor, now := state.actor, b.elapsedMS
	if !actor.Config.GuardEnabled || actor.Config.Team != pb.BattleTeam_BATTLE_TEAM_ATTACKER ||
		req.GetTargetId() != "" || now < actor.guardReady || b.hasStatus(actor, pb.StatusType_STATUS_TYPE_STUN) {
		return
	}
	b.beginAction()
	actor.guardUntil = now + guardWindowMS
	actor.guardReady = now + guardCooldownMS
	state.ready = max(state.ready, now+b.attackInterval(actor))
	ack.Code = common.ErrCode_OK
	ack.ReadyMs = state.ready
	ack.Action = b.action
	id := req.GetUnitId()
	event := b.emit(pb.BattleEventType_BATTLE_EVENT_TYPE_GUARD_STARTED,
		id, id, guardSkillID, "", 0, actor.HP, actor.HP, eventInfo{}, nil)
	event.UpdatedUnit = actor.healthSnapshot()
	event.UpdatedControl = b.control(actor)
}

// startWindup 锁定重击目标直至命中，收刀后才允许机器人提交下一招。
func (b *Battle) startWindup(state *inputState, action *battleAction) {
	impact := b.elapsedMS + action.Skill.WindupMS
	state.actor.windup = &pb.BattleWindup{
		Action: action.Number, SkillId: action.Skill.ID, TargetId: firstTarget(action.TargetIDs),
		StartMs: b.elapsedMS, ImpactMs: impact,
	}
	state.ready = impact + b.attackInterval(state.actor)
	b.schedule(&timelineItem{ElapsedMs: impact, Phase: phaseStrikeImpact, Actor: state.actor, Strike: action})
}

// landStrike 使用原动作编号在命中时刻统一结算；死亡目标不会被重新选取。
func (b *Battle) landStrike(action *battleAction) {
	previous := b.action
	b.action = action.Number
	action.Actor.windup = nil
	b.impactAction(action)
	b.endAction(action)
	b.action = previous
}

// strikeDamage 计算格挡后的三成伤害或反击的一点五倍伤害，并限制整数边界。
func strikeDamage(amount int64, blocked, counter bool) int64 {
	if amount <= 0 {
		return amount
	}
	if blocked {
		return max(1, amount/10*3+amount%10*3/10)
	}
	if counter {
		return amount + min(math.MaxInt64-amount, amount/2)
	}
	return amount
}

package robot

import (
	"container/heap"
	"time"

	"github.com/gogu-x/gogs/battle/robot/strategy"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// Observe 请求同批到期单位的战况，顺序沿用决策队列的排序。
type Observe struct {
	Requests []*pb.BattleInputReq // 只携带单位和待提交的序号。
}

// DecisionView 是战斗 Actor 返回的只读快照，不共享引擎的可变状态。
type DecisionView struct {
	Requests []*pb.BattleInputReq                // 对应的决策批次。
	Units    []*pb.BattleUnitResult              // 当前单位的阵营与血量。
	Skills   map[string]map[string]pb.TargetRule // 每个单位能用的技能和目标规则。
}

// observe 仅在本批次需要条件决策时通过邮箱获取战况。
func (r *RobotActor) observe(units []*unitAI, now int64) {
	query := &Observe{}
	for _, unit := range units {
		query.Requests = append(query.Requests, &pb.BattleInputReq{UnitId: unit.control.GetUnitId(), Sequence: unit.sequence + 1})
	}
	if !tree.TrySend(r.target, query) {
		for _, unit := range units {
			r.retryDecision(unit, now)
		}
	}
}

// retryDecision 将单位排到下一毫秒重新决策。
func (r *RobotActor) retryDecision(unit *unitAI, now int64) {
	unit.due = now + 1
	heap.Push(&r.queue, unit)
}

// applyView 在机器人邮箱内执行策略，已死亡或已经提交过的单位跳过。
func (r *RobotActor) applyView(view *DecisionView) {
	now := time.Since(r.start).Milliseconds()
	for _, req := range view.Requests {
		id := req.GetUnitId()
		unit := r.units[id]
		if unit == nil || unit.waiting || unit.index >= 0 || req.GetSequence() != unit.sequence+1 {
			continue
		}
		var skill, target string
		if len(unit.rules) > 0 {
			skill, target = strategy.Select(unit.control, unit.rules, view.Units, view.Skills[id])
			if skill == "" {
				delete(r.units, id)
				continue
			}
		} else {
			skill = unit.policy(unit.control, unit.last)
		}
		r.submit(unit, skill, target, now)
	}
}

// submit 只提交技能和目标；校验、扣血和冷却变更由战斗 Actor 串行执行。
func (r *RobotActor) submit(unit *unitAI, skill, target string, now int64) {
	input := &pb.BattleInputReq{BattleId: r.battleID, UnitId: unit.control.GetUnitId(), Sequence: unit.sequence + 1, SkillId: skill, TargetId: target}
	if !tree.TrySend(r.target, &Command{Input: input}) {
		r.retryDecision(unit, now)
		return
	}
	unit.sequence++
	unit.waiting = true
}

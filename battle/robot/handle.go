package robot

import (
	"container/heap"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// registerMessages 注册会改变单位调度状态的消息。未知消息沿用路由默认的忽略行为。
func (r *RobotActor) registerMessages() {
	tree.RegisterFunc(&r.router, (*pb.BattleInputAck)(nil), r.onInputAck)
	tree.RegisterFunc(&r.router, (*DecisionView)(nil), r.onDecisionView)
	tree.RegisterFunc(&r.router, (*UnitRemoved)(nil), r.onUnitRemoved)
}

// onInputAck 只接受当前等待的回执，并将对应单位排入下一次决策。
func (r *RobotActor) onInputAck(_ tree.Context, msg *pb.BattleInputAck) {
	unit := r.units[msg.GetUnitId()]
	if msg.GetBattleId() != r.battleID || unit == nil || !unit.waiting || unit.sequence != msg.GetSequence() {
		return
	}
	unit.last = msg
	unit.waiting = false
	unit.due = max(msg.GetReadyMs(), msg.GetCurrentMs()+1)
	heap.Push(&r.queue, unit)
	r.arm()
}

// onDecisionView 应用战况快照后重新检查最近的唤醒时间。
func (r *RobotActor) onDecisionView(_ tree.Context, msg *DecisionView) { r.applyView(msg); r.arm() }

// onUnitRemoved 移除单位及其待执行决策，再按剩余队列安排唤醒。
func (r *RobotActor) onUnitRemoved(_ tree.Context, msg *UnitRemoved) {
	id := msg.UnitID
	if unit := r.units[id]; unit != nil {
		if unit.index >= 0 {
			heap.Remove(&r.queue, unit.index)
		}
		delete(r.units, id)
	}
	r.arm()
}

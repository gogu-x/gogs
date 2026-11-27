package engine

import "container/heap"

// unitLess 按站位和实例编号确定单位先后顺序。
func unitLess(left, right *unit) bool {
	if left.Config.Position != right.Config.Position {
		return left.Config.Position < right.Config.Position
	}
	return left.Config.InstanceID < right.Config.InstanceID
}

type timelinePhase uint8

const (
	// 状态先到期，再执行周期效果及动作，避免到期时刻继续享受增益。
	phaseStatusExpire timelinePhase = iota
	// 周期效果在该时刻动作之前执行，按回合效果仍沿用原有时机。
	phaseStatusTick
	// 已到期的重击在同刻状态任务后、新输入前结算。
	phaseStrikeImpact
)

type timelineItem struct {
	ElapsedMs int64
	Phase     timelinePhase
	Actor     *unit
	Strike    *battleAction // 重击任务持有举刀时锁定的技能和目标。

	StatusID      uint64 // 状态任务关联的实例编号。
	StatusVersion uint64 // 入队时的版本，刷新后旧任务自动失效。
}

type timelineQueue []*timelineItem

// Len 返回堆中待调度项的数量。
func (q timelineQueue) Len() int { return len(q) }

// Less 比较两个待调度项的执行优先级。
func (q timelineQueue) Less(i, j int) bool {
	left, right := q[i], q[j]
	if left.ElapsedMs != right.ElapsedMs {
		return left.ElapsedMs < right.ElapsedMs
	}
	if left.Phase != right.Phase {
		return left.Phase < right.Phase
	}
	if left.Actor != right.Actor {
		return unitLess(left.Actor, right.Actor)
	}
	if left.StatusID != right.StatusID {
		return left.StatusID < right.StatusID
	}
	return left.StatusVersion < right.StatusVersion
}

// Swap 交换两个堆元素的位置。
func (q timelineQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

// Push 将新调度项追加到堆尾。
func (q *timelineQueue) Push(value any) { *q = append(*q, value.(*timelineItem)) }

// Pop 取出并移除堆尾的调度项。
func (q *timelineQueue) Pop() any {
	old := *q
	last := old[len(old)-1]
	*q = old[:len(old)-1]
	return last
}

// schedule 将待执行事件加入时间线。
func (b *Battle) schedule(item *timelineItem) { heap.Push(&b.timeline, item) }

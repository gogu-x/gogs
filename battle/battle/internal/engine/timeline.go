package engine

import "container/heap"

func unitLess(left, right *unit) bool {
	if left.Config.Position != right.Config.Position {
		return left.Config.Position < right.Config.Position
	}
	return left.Config.InstanceID < right.Config.InstanceID
}

type timelinePhase uint8

const (
	// 状态先到期，再执行周期效果及动作，避免到期帧继续享受增益。
	phaseStatusExpire timelinePhase = iota
	// 周期效果在该帧动作之前执行，按回合效果仍沿用原有时机。
	phaseStatusTick
	// 已在更早 tick 开始的动作先结算 impact，再结束，最后允许新动作开始。
	// 零前摇动作在 start 处理时插入 impact，仍会在同 tick 确定性结算。
	phaseImpact
	// 结束当前动作并结算回合状态。
	phaseEnd
	// 开始单位的下一次行动。
	phaseStart
)

type scheduledAction struct {
	ID         uint64
	Actor      *unit
	Skill      SkillConfig
	Targets    []*unit
	TargetIDs  []string
	StartTick  int64
	ImpactTick int64
	EndTick    int64
	Basic      bool
}

type timelineItem struct {
	Tick          int64
	Phase         timelinePhase
	Actor         *unit
	Action        *scheduledAction
	StatusID      uint64 // 状态任务关联的实例编号。
	StatusVersion uint64 // 入队时的版本，刷新后旧任务自动失效。
}

type timelineQueue []*timelineItem

func (q timelineQueue) Len() int { return len(q) }
func (q timelineQueue) Less(i, j int) bool {
	left, right := q[i], q[j]
	if left.Tick != right.Tick {
		return left.Tick < right.Tick
	}
	if left.Phase != right.Phase {
		return left.Phase < right.Phase
	}
	leftAction, rightAction := uint64(0), uint64(0)
	if left.Action != nil {
		leftAction = left.Action.ID
	}
	if right.Action != nil {
		rightAction = right.Action.ID
	}
	if leftAction != rightAction {
		return leftAction < rightAction
	}
	if left.Actor != right.Actor {
		return unitLess(left.Actor, right.Actor)
	}
	if left.StatusID != right.StatusID {
		return left.StatusID < right.StatusID
	}
	return left.StatusVersion < right.StatusVersion
}
func (q timelineQueue) Swap(i, j int)   { q[i], q[j] = q[j], q[i] }
func (q *timelineQueue) Push(value any) { *q = append(*q, value.(*timelineItem)) }
func (q *timelineQueue) Pop() any {
	old := *q
	last := old[len(old)-1]
	*q = old[:len(old)-1]
	return last
}

func (b *Battle) schedule(item *timelineItem) { heap.Push(&b.timeline, item) }

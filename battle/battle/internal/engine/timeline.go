package engine

import "container/heap"

type timelinePhase uint8

const (
	// 已在更早 tick 开始的动作先结算 impact，再结束，最后允许新动作开始。
	// 零前摇动作在 start 处理时插入 impact，仍会在同 tick 确定性结算。
	phaseImpact timelinePhase = iota
	phaseEnd
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
	Tick   int64
	Phase  timelinePhase
	Actor  *unit
	Action *scheduledAction
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
	return unitLess(left.Actor, right.Actor)
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

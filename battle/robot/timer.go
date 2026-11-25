package robot

import (
	"container/heap"
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// decisionTimer 唤醒机器人提交下一次动作。
const decisionTimer = timer.TimerType(1)

func (r *RobotActor) initTimer(pid tree.PID) {
	r.wheel = timer.NewTimeWheel(1, pid)
	r.wheel.Register(decisionTimer, r.decide)
	r.arm()
}

// arm 始终只保留最早决策帧的一个唤醒任务。
func (r *RobotActor) arm() {
	if len(r.queue) == 0 {
		r.wake.Stop()
		r.wake = nil
		return
	}
	next := r.queue[0].due
	if r.wake != nil && r.wakeTick == next {
		return
	}
	r.wake.Stop()
	r.wakeTick = next
	delay := time.Until(r.start.Add(time.Duration(next) * r.frame))
	r.wake = r.wheel.After(decisionTimer, delay, nil)
}

// decide 批量提交已到期单位的动作，未到期单位保留原计划。
func (r *RobotActor) decide(interface{}) {
	r.wake = nil
	now := int64(time.Since(r.start) / r.frame)
	var due []*unitAI
	observe := false
	for len(r.queue) > 0 && r.queue[0].due <= now {
		unit := heap.Pop(&r.queue).(*unitAI)
		due = append(due, unit)
		observe = observe || len(unit.rules) > 0
	}
	if observe {
		r.observe(due, now)
	} else {
		for _, unit := range due {
			r.submit(unit, unit.policy(unit.control, unit.last), "", now)
		}
	}
	r.arm()
}

type decisionQueue []*unitAI

func (q decisionQueue) Len() int { return len(q) }
func (q decisionQueue) Less(i, j int) bool {
	a, b := q[i], q[j]
	if a.due != b.due {
		return a.due < b.due
	}
	if a.position != b.position {
		return a.position < b.position
	}
	return a.control.GetUnitId() < b.control.GetUnitId()
}
func (q decisionQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *decisionQueue) Push(v any)   { unit := v.(*unitAI); unit.index = len(*q); *q = append(*q, unit) }
func (q *decisionQueue) Pop() any {
	items := *q
	last := len(items) - 1
	unit := items[last]
	items[last] = nil
	*q = items[:last]
	unit.index = -1
	return unit
}

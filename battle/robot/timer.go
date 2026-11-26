package robot

import (
	"container/heap"
	"time"

	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// decisionTimer 唤醒机器人提交下一次动作。
const decisionTimer = timer.TimerType(1)

// initTimer 初始化调度器的定时唤醒。
func (r *RobotActor) initTimer(pid tree.PID) {
	r.wheel = timer.NewTimeWheel(1, pid)
	r.wheel.Register(decisionTimer, r.decide)
	r.arm()
}

// arm 始终只保留最早决策时间的一个唤醒任务。
func (r *RobotActor) arm() {
	if len(r.queue) == 0 {
		r.wake.Stop()
		r.wake = nil
		return
	}
	next := r.queue[0].due
	if r.wake != nil && r.wakeMS == next {
		return
	}
	r.wake.Stop()
	r.wakeMS = next
	delay := time.Until(r.start.Add(time.Duration(next) * time.Millisecond))
	r.wake = r.wheel.After(decisionTimer, delay, nil)
}

// decide 批量提交已到期单位的动作，未到期单位保留原计划。
func (r *RobotActor) decide(interface{}) {
	r.wake = nil
	now := time.Since(r.start).Milliseconds()
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

// Len 返回堆中待调度项的数量。
func (q decisionQueue) Len() int { return len(q) }

// Less 比较两个待调度项的执行优先级。
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

// Swap 交换两个堆元素的位置。
func (q decisionQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }

// Push 将新调度项追加到堆尾。
func (q *decisionQueue) Push(v any) { unit := v.(*unitAI); unit.index = len(*q); *q = append(*q, unit) }

// Pop 取出并移除堆尾的调度项。
func (q *decisionQueue) Pop() any {
	items := *q
	last := len(items) - 1
	unit := items[last]
	items[last] = nil
	*q = items[:last]
	unit.index = -1
	return unit
}

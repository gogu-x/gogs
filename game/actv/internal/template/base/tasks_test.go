package base

import (
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"testing"
)

// newTestTaskComp 创建测试用个人任务组件。
func newTestTaskComp() *TaskComp {
	return NewTaskComp(2, &service.ActvMeta{ActvID: 10, OwnerID: 7, OwnerScope: service.ScopePlayer})
}

// TestTaskCompLifecycle 验证独立存盘、类型恢复、玩家隔离、标脏及损坏存档阻断。
func TestTaskCompLifecycle(t *testing.T) {
	tlog.NewLog("", 0)
	first := newTestTaskComp()
	first.OpenTasks()
	c := first.Tasks()
	if c.Add(task.Instance{ID: 1, Goal: 0, Target: 3}, 1) {
		t.Fatal("接受零类型")
	}
	if !c.Add(task.Instance{ID: 1, ConfigID: 1, Goal: 91, Target: 3}, 1) {
		t.Fatal("添加失败")
	}
	if !first.IsDirty() {
		t.Fatal("添加任务未标脏")
	}
	raw, err := first.Save()
	if err != nil {
		t.Fatal(err)
	}
	second := newTestTaskComp()
	if err := second.Load(raw); err != nil {
		t.Fatal(err)
	}
	otherHub := &task.Hub{}
	otherHub.Publish(task.Event{Goal: 91, Count: 1}, 1)
	if second.IsDirty() {
		t.Fatal("其他玩家修改任务")
	}
	second.OpenTasks()
	hub := &task.Hub{}
	second.AttachTasks(hub)
	hub.Publish(task.Event{Goal: 91, Count: 1}, 1)
	if !second.IsDirty() || second.GetData().GetTasks()[1].GetProgress() != 1 || first.GetData().GetTasks()[1].GetProgress() != 0 {
		t.Fatal("恢复或实例隔离错误")
	}
	snapshot := second.BindSnapshot()
	snapshot.Publish(task.Event{Goal: 91, Count: 1}, 1)
	snapshot.Close()
	if second.GetData().GetTasks()[1].GetProgress() != 1 {
		t.Fatal("快照修改真实进度")
	}
	if err := second.Load([]byte{1}); err == nil {
		t.Fatal("无效数据未阻断")
	}
	if second.Tasks() != nil {
		t.Fatal("阻断组件仍可写")
	}
}

// TestCommonTaskHandler 验证参数过滤、自动订阅及目标封顶。
func TestCommonTaskHandler(t *testing.T) {
	tlog.NewLog("", 0)
	cases := []struct {
		name               string
		params             []int64
		param, count, want int64
	}{
		{"无条件", nil, 99, 2, 2}, {"匹配", []int64{7, 8}, 8, 2, 2},
		{"不匹配", []int64{7, 8}, 9, 2, 0}, {"封顶", nil, 0, 100, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestTaskComp()
			m.OpenTasks()
			hub := &task.Hub{}
			m.AttachTasks(hub)
			c := m.Tasks()
			if !c.Add(task.Instance{ID: 1, Goal: 99, Target: 5, Params: tc.params}, 1) {
				t.Fatal("添加失败")
			}
			m.ClearDirty(m.Revision())
			hub.Publish(task.Event{Goal: 99, Param: tc.param, Count: tc.count}, 1)
			if m.GetData().GetTasks()[1].GetProgress() != tc.want || m.IsDirty() != (tc.want > 0) {
				t.Fatal("进度或标脏错误")
			}
		})
	}
}

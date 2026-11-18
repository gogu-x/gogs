package service_test

import (
	"testing"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
)

// ownerTaskBehavior 为归属隔离测试提供真实任务存盘组件。
type ownerTaskBehavior struct {
	base.BaseBehavior
	tasks *base.TaskComp
}

// Create 为每个归属创建独立模板和任务组件。
func (*ownerTaskBehavior) Create(a *service.Actv, cfg conf.ActvConfig) service.ActvBehavior {
	b := &ownerTaskBehavior{tasks: base.NewTaskComp(1, a.GetMeta())}
	b.Bind(a, cfg)
	b.AddComp(b.tasks)
	return b
}

// TestTaskOwnerIsolation 验证相同编号的三种归属隔离、结束注销和玩家卸载不影响共享任务。
func TestTaskOwnerIsolation(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(7)
	svc.SetPlayerLoaded(7)
	scopes := []service.OwnerScope{service.ScopePlayer, service.ScopeGuild, service.ScopeServer, service.ScopeGuild}
	states := make([]*task.State, len(scopes))
	for i, scope := range scopes {
		typ := service.TemplateType(i + 1)
		svc.RegTpl(typ, &ownerTaskBehavior{}, scope)
		id := service.OwnerID(7)
		if i == 3 {
			id = 8
		}
		a := svc.NewActv(&service.ActvMeta{ActvID: int64(i + 1), OwnerScope: scope, OwnerID: id, TemplateType: typ}, conf.ActvConfig{})
		if !svc.AddActv(a) {
			t.Fatal("创建活动失败")
		}
		svc.Loaded(a)
		b := a.GetExt().(*ownerTaskBehavior)
		if !b.tasks.Tasks().Add(task.Instance{ID: 1, Goal: 4, Target: 20}, 1) {
			t.Fatal("添加任务失败")
		}
		states[i] = b.tasks.GetData()
	}
	progress := task.Event{Goal: 4, Count: 1}
	messages := []*sspb.Event{gameevent.NewTask(7, progress), gameevent.NewGuildTask(7, progress), gameevent.NewServerTask(progress)}
	for i, message := range messages {
		svc.HandleEvent(message)
		for j, state := range states {
			want := int64(0)
			if j <= i {
				want = 1
			}
			if state.GetTasks()[1].GetProgress() != want {
				t.Fatalf("归属串事件: event=%d owner=%d", i, j)
			}
		}
	}
	svc.UnloadPlayer(7, 7)
	for _, message := range messages {
		svc.HandleEvent(message)
	}
	if states[0].GetTasks()[1].GetProgress() != 1 || states[1].GetTasks()[1].GetProgress() != 2 || states[2].GetTasks()[1].GetProgress() != 2 {
		t.Fatal("玩家卸载影响共享Hub")
	}
	svc.EndActv(2)
	svc.HandleEvent(messages[1])
	if states[1].GetTasks()[1].GetProgress() != 2 {
		t.Fatal("联盟活动结束后仍收到事件")
	}
	svc.RemoveActv(3)
	svc.HandleEvent(messages[2])
	if states[2].GetTasks()[1].GetProgress() != 2 {
		t.Fatal("服务器活动移除后仍收到事件")
	}
}

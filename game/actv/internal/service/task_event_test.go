package service_test

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"testing"
	"time"
)

// taskObserver 只挂载任务数据，不实现任何任务类型事件处理函数。
type taskObserver struct {
	base.BaseBehavior
	storage *base.TaskComp
}

func (b *taskObserver) Create(a *service.Actv, c conf.ActvConfig) service.ActvBehavior {
	b.Bind(a, c)
	return b
}

// TestTaskBroadcast 验证一次事件同时推进两个活动，并过滤其他玩家、条件、类型和关闭活动。
func TestTaskBroadcast(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	svc.SetPlayerLoaded(7)
	svc.SetPlayerLoaded(8)
	observers := make([]*taskObserver, 7)
	for i := range observers {
		b := &taskObserver{}
		uid := service.OwnerID(7)
		if i == 2 {
			uid = 8
		}
		meta := &service.ActvMeta{ActvID: int64(i + 1), OwnerID: uid, OwnerScope: service.ScopePlayer}
		if i == 5 {
			meta.State = service.StateEnded
		}
		if i == 6 {
			meta.StartTs = time.Now().Unix() + 3600
		}
		b.storage = base.NewTaskComp(1, meta)
		b.AddComp(b.storage)
		svc.RegTpl(1, b, service.ScopePlayer)
		meta.TemplateType = 1
		actv := svc.NewActv(meta, conf.ActvConfig{})
		if !svc.AddActv(actv) {
			t.Fatal("创建失败")
		}
		svc.Loaded(actv)
		goal := uint32(4)
		param := int64(99)
		if i == 3 {
			goal = 5
		}
		if i == 4 {
			param = 100
		}
		c := b.storage.Tasks()
		if !c.Add(task.Instance{ID: 1, Goal: goal, Target: 1, Params: []int64{param}}, time.Now().Unix()) {
			t.Fatal("添加失败")
		}
		b.storage.ClearDirty(b.storage.Revision())
		observers[i] = b
	}
	receiver := &taskReceiver{service: svc, done: make(chan struct{}, 1)}
	pid := tree.SpawnOne(receiver)
	t.Cleanup(func() { tree.Send(pid, struct{}{}) })
	if !gameevent.PublishTask(7, task.Event{Goal: 4, Param: 99, Count: 1}) {
		t.Fatal("事件未投递")
	}
	select {
	case <-receiver.done:
	case <-time.After(time.Second):
		t.Fatal("事件未处理")
	}

	for i, b := range observers {
		want := int64(0)
		if i < 2 {
			want = 1
		}
		instance := b.storage.GetData().GetTasks()[1]
		if instance.GetProgress() != want || b.storage.IsDirty() != (want > 0) {
			t.Fatalf("活动%d进度或标脏错误", i)
		}
		if i < 2 && instance.GetStatus() != task.Finished {
			t.Fatal("两个订阅活动未同时完成")
		}
	}
	svc.HandleEvent(gameevent.NewTask(7, task.Event{Goal: 4, Param: 99, Count: 1}))
	if observers[0].storage.Revision() != 2 || observers[1].storage.Revision() != 2 {
		t.Fatal("已完成任务仍推进")
	}
}

// taskReceiver 在测试Actor中复用生产公共分发入口，验证发布方无需知道订阅者。
type taskReceiver struct {
	service *service.ActvService
	done    chan struct{}
}

func (*taskReceiver) Name() string        { return def.ActorActv }
func (*taskReceiver) OnInit(tree.Context) {}
func (*taskReceiver) OnStop(tree.Context) {}
func (a *taskReceiver) HandleMessage(ctx tree.Context, value interface{}) {
	if message, ok := value.(*sspb.Event); ok {
		a.service.HandleEvent(message)
		a.done <- struct{}{}
	} else {
		ctx.Stop()
	}
}

// TestTaskHubLifecycle 验证重复加载、活动结束、玩家卸载和重新加载不会残留或重复订阅。
func TestTaskHubLifecycle(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	svc.SetPlayerLoaded(7)
	create := func(id int64, raw []byte) (*service.Actv, *taskObserver) {
		meta := &service.ActvMeta{ActvID: id, OwnerID: 7, OwnerScope: service.ScopePlayer, TemplateType: 1}
		b := &taskObserver{storage: base.NewTaskComp(1, meta)}
		b.AddComp(b.storage)
		svc.RegTpl(1, b, service.ScopePlayer)
		a := svc.NewActv(meta, conf.ActvConfig{})
		if !svc.AddActv(a) {
			t.Fatal("创建失败")
		}
		if len(raw) > 0 {
			if err := b.storage.Load(raw); err != nil {
				t.Fatal(err)
			}
		}
		svc.Loaded(a)
		if len(raw) == 0 {
			if !b.storage.Tasks().Add(task.Instance{ID: 1, Goal: 4, Target: 10}, 1) {
				t.Fatal("添加失败")
			}
		}
		return a, b
	}
	first, a := create(101, nil)
	_, b := create(102, nil)
	publish := func() { svc.HandleEvent(gameevent.NewTask(7, task.Event{Goal: 4, Count: 1})) }
	svc.Loaded(first)
	publish()
	if a.storage.GetData().GetTasks()[1].GetProgress() != 1 || b.storage.GetData().GetTasks()[1].GetProgress() != 1 {
		t.Fatal("重复加载造成重复订阅")
	}
	if !svc.EndActv(101) || a.storage.Tasks() != nil {
		t.Fatal("活动结束未关闭任务组件")
	}
	publish()
	if a.storage.GetData().GetTasks()[1].GetProgress() != 1 || b.storage.GetData().GetTasks()[1].GetProgress() != 2 {
		t.Fatal("结束后仍收到事件")
	}
	raw, err := b.storage.Save()
	if err != nil {
		t.Fatal(err)
	}
	svc.UnloadPlayer(7, 0)
	if b.storage.Tasks() != nil {
		t.Fatal("卸载未关闭任务组件")
	}
	publish()
	if b.storage.GetData().GetTasks()[1].GetProgress() != 2 {
		t.Fatal("卸载后仍收到事件")
	}
	svc.SetPlayerLoaded(7)
	_, restored := create(102, raw)
	publish()
	if restored.storage.GetData().GetTasks()[1].GetProgress() != 3 || b.storage.GetData().GetTasks()[1].GetProgress() != 2 {
		t.Fatal("重载残留旧订阅或未恢复进度")
	}
}

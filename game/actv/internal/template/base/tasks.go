package base

import (
	"fmt"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"sort"
)

// TaskComp 是独立任务存盘组件，拥有任务进度、公共计数规则及变化推送。
// 通过AddComp注册，按现有活动组件生命周期加载、标脏和保存。
type TaskComp struct {
	*Comp[task.State]                   // 任务存档和存盘版本管理。
	meta              *service.ActvMeta // 活动归属。
	registry          *task.Registry    // 公共计数规则。
	component         *task.Component   // 随数据生命周期持有的任务组件。
	hub               *task.Hub         // 所属玩家的任务分发入口。
}

// NewTaskComp 创建独立任务组件，类型由每个任务的Goal声明。
func NewTaskComp(id int32, meta *service.ActvMeta) *TaskComp {
	goals, err := task.NewGoalsWithDefault(accumulateTask)
	if err != nil {
		panic(err)
	}
	registry, err := task.NewRegistry(task.System(fmt.Sprintf("actv-tasks-%d", meta.GetCfgID())), goals, taskPushHooks{})
	if err != nil {
		panic(err)
	}
	return &TaskComp{Comp: NewComp[task.State](id), meta: meta, registry: registry}
}

// OpenTasks 在数据加载完成后恢复任务组件，重复调用不会重复绑定。
func (m *TaskComp) OpenTasks() {
	if m.component != nil || m.Blocked() {
		return
	}
	component, _, err := task.Bind(m.registry, m.GetData(), m)
	if err != nil {
		tlog.Log.Error("[actv/TaskComp.OpenTasks] 恢复任务失败, actv=%v err=%v", m.meta.GetActvID(), err)
		return
	}
	m.component = component
}

// Tasks 返回生命周期持有的任务组件，业务调用方不得关闭。
func (m *TaskComp) Tasks() *task.Component { return m.component }

// AttachTasks 在活动开放时挂到所属玩家Hub，重复挂载无副作用。
func (m *TaskComp) AttachTasks(hub *task.Hub) {
	if hub == nil || m.component == nil || m.hub == hub {
		return
	}
	if m.hub != nil {
		m.hub.Detach(m.component)
	}
	if hub.Attach(m.component) {
		m.hub = hub
	}
}

// CloseTasks 先卸载订阅，再关闭组件，阻止结束或释放后继续推进任务。
func (m *TaskComp) CloseTasks() {
	if m.hub != nil {
		m.hub.Detach(m.component)
		m.hub = nil
	}
	if m.component != nil {
		m.component.Close()
		m.component = nil
	}
}

// Load 在重新加载存档前释放旧运行态，加载完成后由活动生命周期重新打开。
func (m *TaskComp) Load(raw bson.Raw) error { m.CloseTasks(); return m.Comp.Load(raw) }

// BindSnapshot 打开不影响真实进度的操作快照，用于异步发奖前的业务校验。
func (m *TaskComp) BindSnapshot() *task.Component {
	if m.Blocked() {
		return nil
	}
	component, _, err := task.Bind(m.registry, m.GetData().DeepCopy(), nil)
	if err != nil {
		return nil
	}
	return component
}

// accumulateTask 按任务配置匹配事件参数，再使用通用累计规则推进进度。
func accumulateTask(current task.Instance, event task.Event) (int64, bool) {
	params := current.GetParams()
	matched := len(params) == 0
	for _, v := range params {
		if v == event.GetParam() {
			matched = true
			break
		}
	}
	if !matched {
		return current.GetProgress(), false
	}
	return task.Accumulate(current, event)
}

// taskPushHooks 在任务变化后统一发送当前状态，模板不参与事件转发。
type taskPushHooks struct{ task.BaseHooks }

// Changed 仅在组件已挂载时推送任务变化。
func (taskPushHooks) Changed(owner task.Owner, _ task.Change) {
	m, ok := owner.(*TaskComp)
	if !ok || m.hub == nil {
		return
	}
	message := &pb_actv.ActvTasksNtf{ActvId: uint64(m.meta.GetActvID())}
	for _, instance := range m.GetData().GetTasks() {
		message.Tasks = append(message.Tasks, &pb_actv.ActvTaskState{TaskId: int32(instance.GetConfigID()), Progress: instance.GetProgress(), Target: instance.GetTarget(), Status: int32(instance.GetStatus())})
	}
	sort.Slice(message.Tasks, func(i, j int) bool { return message.Tasks[i].GetTaskId() < message.Tasks[j].GetTaskId() })
	ipb.PushToPlayer(uint64(m.meta.GetOwnerID()), message)
}

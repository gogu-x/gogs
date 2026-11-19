package service

import "github.com/gogu-x/tree/task"
import "github.com/gogu-x/tree/comm"

// taskLifecycle 由任务组件实现，挂载和卸载均由活动生命周期管理。
type taskLifecycle interface {
	OpenTasks()
	AttachTasks(*task.Hub)
	CloseTasks()
}

// Loaded 在全部组件存档恢复完成后初始化运行态，再通知模板并挂载订阅。
func (as *ActvService) Loaded(actv *Actv) {
	if actv == nil || actv.dataReady {
		return
	}
	for _, comp := range actv.comps {
		if tasks, ok := comp.(taskLifecycle); ok {
			tasks.OpenTasks()
		}
	}
	actv.dataReady = true
	actv.GetExt().OnLoaded()
	as.attachTasks(actv, comm.NowUnix())
}

// attachTasks 为已加载且当前开放的活动按归属挂载任务组件。
func (as *ActvService) attachTasks(actv *Actv, now int64) {
	meta := actv.GetMeta()
	owner := actv.ownerKey()
	if !actv.dataReady || !as.taskOwnerReady(owner) || actv.IsEnded() || meta.GetStartTs() > now || (meta.GetEndTs() > 0 && meta.GetEndTs() <= now) {
		return
	}
	for _, comp := range actv.comps {
		if tasks, ok := comp.(taskLifecycle); ok {
			hub := as.taskHubs[owner]
			if hub == nil {
				hub = &task.Hub{}
				as.taskHubs[owner] = hub
			}
			tasks.AttachTasks(hub)
		}
	}
}

// closeTasks 在活动结束、移除和玩家卸载时注销任务订阅。
func (actv *Actv) closeTasks() {
	for _, comp := range actv.comps {
		if tasks, ok := comp.(taskLifecycle); ok {
			tasks.CloseTasks()
		}
	}
}

// publishTasks 只通过对应归属Hub分发事件，活动时间边界由生命周期定时器推进。
func (as *ActvService) publishTasks(owner ownerKey, progress task.Event, now int64) {
	if !as.taskOwnerReady(owner) {
		return
	}
	if hub := as.taskHubs[owner]; hub != nil {
		hub.Publish(progress, now)
	}
}

// taskOwnerReady 校验归属，只有玩家归属需要等待玩家加载。
func (as *ActvService) taskOwnerReady(owner ownerKey) bool {
	if owner.id <= 0 {
		return false
	}
	switch owner.scope {
	case ScopePlayer:
		return as.IsPlayerLoaded(uint64(owner.id))
	case ScopeGuild:
		return true
	case ScopeServer:
		return owner.id == as.serverID
	default:
		return false
	}
}

package service

import "github.com/gogu-x/tree/task"
import "time"

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
	as.attachTasks(actv, time.Now().Unix())
}

// attachTasks 只为已加载且当前开放的个人活动挂载任务组件。
func (as *ActvService) attachTasks(actv *Actv, now int64) {
	meta := actv.GetMeta()
	uid := uint64(meta.GetOwnerID())
	if !actv.dataReady || meta.GetOwnerScope() != ScopePlayer || !as.IsPlayerLoaded(uid) || actv.IsEnded() || meta.GetStartTs() > now || (meta.GetEndTs() > 0 && meta.GetEndTs() <= now) {
		return
	}
	for _, comp := range actv.comps {
		if tasks, ok := comp.(taskLifecycle); ok {
			hub := as.taskHubs[uid]
			if hub == nil {
				hub = &task.Hub{}
				as.taskHubs[uid] = hub
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

// publishTasks 先推进该玩家活动的时间边界，然后由Hub统一分发一次事件。
func (as *ActvService) publishTasks(uid uint64, progress task.Event, now int64) {
	if !as.IsPlayerLoaded(uid) {
		return
	}
	for _, actv := range as.ListActvs(ScopePlayer, OwnerID(uid)) {
		meta := actv.GetMeta()
		if !actv.dataReady || actv.IsEnded() {
			continue
		}
		if meta.GetEndTs() > 0 && meta.GetEndTs() <= now {
			as.EndActv(meta.GetActvID())
			continue
		}
		if meta.GetStartTs() > now {
			continue
		}
		if !meta.Started {
			actv.GetExt().OnStart()
			meta.Started = true
			meta.MarkDirty()
			as.attachTasks(actv, now)
		}
		actv.GetExt().OnTicker()
	}
	if hub := as.taskHubs[uid]; hub != nil {
		hub.Publish(progress, now)
	}
}

package event

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/task"
	"strconv"
)

// NameTaskProgress 是统一任务进度事件，业务参数全部使用扩展字段。
const NameTaskProgress = "taskProgress"

// NewTask 构造玩家任务事件，发布者只需提供任务类型、数量和条件参数。
func NewTask(uid uint64, progress task.Event) *sspb.Event {
	message := NewPlayer(NameTaskProgress, uid, 0)
	message.ExtArgs["goal"] = strconv.FormatUint(uint64(progress.Goal), 10)
	message.ExtArgs["count"] = strconv.FormatInt(progress.Count, 10)
	message.ExtArgs["param"] = strconv.FormatInt(progress.Param, 10)
	return message
}

// ReadTask 读取服务端任务事件，拒绝无效归属、类型、数量和参数。
func ReadTask(message *sspb.Event) (task.Event, bool) {
	if message == nil || message.GetEventTyp() != TypePlayerLifecycle || message.GetName() != NameTaskProgress || PlayerUID(message) == 0 {
		return task.Event{}, false
	}
	args := message.GetExtArgs()
	goal, errGoal := strconv.ParseUint(args["goal"], 10, 32)
	count, errCount := strconv.ParseInt(args["count"], 10, 64)
	param, errParam := strconv.ParseInt(args["param"], 10, 64)
	if errGoal != nil || errCount != nil || errParam != nil || goal == 0 || count <= 0 {
		return task.Event{}, false
	}
	return task.Event{Goal: uint32(goal), Count: count, Param: param}, true
}

// PublishTask 向活动Actor发布一次任务事件，由公共层分发给该玩家所有订阅活动。
// 只能由服务端可信结算调用；成功表示入队，不表示任务已经完成。
func PublishTask(uid uint64, progress task.Event) bool {
	message := NewTask(uid, progress)
	if _, ok := ReadTask(message); !ok {
		return false
	}
	pid, ok := tree.Lookup(def.ActorActv)
	return ok && tree.Send(pid, message)
}

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
	return fillTask(message, progress)
}

// NewGuildTask 构造指定联盟的公共任务事件。
func NewGuildTask(guildID uint64, progress task.Event) *sspb.Event {
	message := &sspb.Event{Name: NameTaskProgress, EventTyp: TypeGuild, ExtArgs: map[string]string{
		ParamGuildID: strconv.FormatUint(guildID, 10),
	}}
	return fillTask(message, progress)
}

// NewServerTask 构造本服公共任务事件，服务器归属由接收活动服务确定。
func NewServerTask(progress task.Event) *sspb.Event {
	return fillTask(&sspb.Event{Name: NameTaskProgress, EventTyp: TypeServerLifecycle, ExtArgs: make(map[string]string)}, progress)
}

// fillTask 写入三种归属共用的任务参数。
func fillTask(message *sspb.Event, progress task.Event) *sspb.Event {
	args := message.GetExtArgs()
	args["goal"] = strconv.FormatUint(uint64(progress.GetGoal()), 10)
	args["count"] = strconv.FormatInt(progress.GetCount(), 10)
	args["param"] = strconv.FormatInt(progress.GetParam(), 10)
	return message
}

// ReadTask 读取服务端任务事件，拒绝无效归属、类型、数量和参数。
func ReadTask(message *sspb.Event) (task.Event, bool) {
	if message == nil || message.GetName() != NameTaskProgress || !validTaskOwner(message) {
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
	return publishTask(NewTask(uid, progress))
}

// PublishGuildTask 发布指定联盟的公共任务事件。
func PublishGuildTask(guildID uint64, progress task.Event) bool {
	return publishTask(NewGuildTask(guildID, progress))
}

// PublishServerTask 发布本服公共任务事件。
func PublishServerTask(progress task.Event) bool { return publishTask(NewServerTask(progress)) }

// publishTask 校验事件后投递到本服活动Actor，成功只表示入队。
func publishTask(message *sspb.Event) bool {
	if _, ok := ReadTask(message); !ok {
		return false
	}
	pid, ok := tree.Lookup(def.ActorActv)
	return ok && tree.Send(pid, message)
}

// validTaskOwner 按事件类型验证归属参数，拒绝无效及超出存档范围的编号。
func validTaskOwner(message *sspb.Event) bool {
	var id uint64
	switch message.GetEventTyp() {
	case TypePlayerLifecycle:
		id = PlayerUID(message)
	case TypeGuild:
		id = Uint64Param(message, ParamGuildID)
	case TypeServerLifecycle:
		return true
	default:
		return false
	}
	return id > 0 && id <= uint64(1<<63-1)
}

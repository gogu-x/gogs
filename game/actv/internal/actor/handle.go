package actor

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

const maxActivityInstanceID = uint64(1<<63 - 1)

// registerHandlers 注册全局活动 Actor 的内部消息处理器。
func (actor *Activity) registerHandlers() {
	tree.RegisterFunc(&actor.router, &sspb.Event{}, actor.onEvent)
	tree.RegisterFunc(&actor.router, &ipb.PlayerActivityMessage{}, actor.onPlayerActivityMessage)
	tree.RegisterFunc(&actor.router, &ipb.SaveAllActivities{}, actor.onSaveAllActivities)
	tree.RegisterFunc(&actor.router, &ipb.PlayerSessionOpened{}, actor.onPlayerSessionOpened)
	tree.RegisterFunc(&actor.router, &ipb.PlayerActivityOwnerReleased{}, actor.onPlayerOwnerReleased)
}

// onPlayerActivityMessage 将玩家活动请求分发到实例绑定的模板行为；玩家数据未就绪时按玩家暂存。
func (actor *Activity) onPlayerActivityMessage(ctx tree.Context, message *ipb.PlayerActivityMessage) {
	if ctx == nil {
		return
	}
	request := ctx.RequestEnvelope()
	if request == nil {
		tlog.Log.Error("[activity/Activity.onPlayerActivityMessage] 收到无请求信封消息, pid=%v", message.GetUID())
		return
	}
	if !actor.ready {
		if len(actor.pending) >= maxPendingActivityRequests {
			request.Respond(nil, fmt.Errorf("activity data is still loading"))
			return
		}
		actor.pending = append(actor.pending, pendingActivityRequest{message: message, request: request})
		return
	}
	if actor.loadErr != nil {
		request.Respond(nil, actor.loadErr)
		return
	}
	actor.dispatchPlayerRequest(ctx, request, message)
}

func (actor *Activity) handlePlayerActivityMessage(message *ipb.PlayerActivityMessage) (interface{}, error) {
	if message == nil {
		return nil, fmt.Errorf("activity request envelope is required")
	}
	uid := message.GetUID()
	activityID := message.GetActvID()
	request := message.GetMessage()
	if uid == 0 || request == nil {
		tlog.Log.Error("[activity/Activity.onPlayerActivityMessage] 活动请求缺少玩家编号或请求体, pid=%v type=%T", uid, request)
		return nil, fmt.Errorf("activity request requires a player and a message")
	}
	tlog.Log.Info("[activity/Activity.onPlayerActivityMessage] 收到玩家活动请求, pid=%v activity=%v type=%T", uid, activityID, request)
	if actor.actvService == nil {
		err := fmt.Errorf("activity service is unavailable")
		tlog.Log.Error("[activity/Activity.onPlayerActivityMessage] 活动服务不可用, pid=%v activity=%v err=%v", uid, activityID, err)
		return nil, err
	}
	actor.actvService.HandleRequest(
		service.ScopePlayer,
		service.OwnerID(uid),
		int64(activityID),
		request,
	)
	return nil, nil
}

// onSaveAllActivities 保存所有活动实例的脏元数据。
func (actor *Activity) onSaveAllActivities(ctx tree.Context, _ *ipb.SaveAllActivities) {
	actor.saveAll(ctx)
}

func (actor *Activity) saveAll(ctx tree.Context) {
	if actor == nil || actor.lifecycle == nil {
		return
	}
	actor.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[activity/Activity.saveAll] 活动数据存盘失败, err=%v", err)
			return
		}
		tlog.Log.Info("[activity/Activity.saveAll] 活动数据存盘完成")
	})
}

// activityList 将玩家当前已加载活动实例打包为客户端活动列表。
func (actor *Activity) activityList(uid uint64) *pb_activity.GetActivityListAck {
	ack := &pb_activity.GetActivityListAck{}
	activities := actor.actvService.ListActvs(service.ScopePlayer, service.OwnerID(uid))
	activityInfos := make([]*pb_activity.ActivityInfo, 0, len(activities))
	for _, actv := range activities {
		if actv == nil {
			continue
		}
		behavior := actv.GetExt()
		if behavior == nil {
			continue
		}
		if info := behavior.OnPack(); info != nil {
			activityInfos = append(activityInfos, info)
		}
	}
	ack.Activities = activityInfos
	return ack
}

// onEvent 将 Actor 收到的事件分发给活动模板并处理服务器生命周期状态。
func (actor *Activity) onEvent(ctx tree.Context, message *sspb.Event) {
	if actor == nil || message == nil {
		return
	}
	if !actor.ready {
		actor.pendingEvents = append(actor.pendingEvents, message)
		return
	}
	actor.dispatchEvent(ctx, message)
}

func (actor *Activity) dispatchEvent(ctx tree.Context, message *sspb.Event) {
	if actor == nil || message == nil {
		return
	}
	if gameevent.IsServerOpened(message) {
		actor.serverOpened = true
		if actor.actvService != nil {
			actor.actvService.StartDueActivities(time.Now().Unix())
		}
	}
	if actor.actvService != nil {
		actor.actvService.HandleEvent(message)
	}
	if gameevent.IsServerOpened(message) {
		tlog.Log.Info("[activity/Activity.onServerEvent] 服务器活动 Actor 已处理开服事件")
		return
	}
	if !gameevent.IsServerClosing(message) {
		return
	}
	actor.serverClosing = true
	tlog.Log.Info("[activity/Activity.onServerEvent] 服务器活动 Actor 已接收关服事件")
	actor.saveAll(ctx)
}

func (actor *Activity) drainPending(ctx tree.Context) {
	if actor == nil {
		return
	}
	events := actor.pendingEvents
	actor.pendingEvents = nil
	for _, event := range events {
		actor.dispatchEvent(ctx, event)
	}
	pending := actor.pending
	actor.pending = nil
	logins := actor.pendingLogins
	actor.pendingLogins = make(map[uint64]bool)
	if actor.loadErr != nil {
		respondPending(pending, actor.loadErr)
		return
	}
	for uid := range logins {
		actor.ensurePlayerLoaded(ctx, uid)
	}
	for _, item := range pending {
		if item.request == nil {
			continue
		}
		message, ok := item.message.(*ipb.PlayerActivityMessage)
		if !ok {
			item.request.Respond(nil, fmt.Errorf("unsupported pending activity request %T", item.message))
			continue
		}
		actor.dispatchPlayerRequest(ctx, item.request, message)
	}
}

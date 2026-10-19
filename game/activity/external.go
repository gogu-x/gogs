package activity

import (
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity/internal"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityowner "github.com/gogu-x/gogs/game/activity/internal/owner"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	// defaultActivityServerID 表示未传入服务器编号时使用的默认编号。
	defaultActivityServerID = uint64(1)
	// activityOwnerSaveInterval 表示活动记录的定时存盘间隔。
	activityOwnerSaveInterval = time.Minute
	// maxPendingActivityRequests 表示活动 Actor 加载服务器数据期间暂存请求的数量上限。
	maxPendingActivityRequests = 1024
)

// pendingActivityRequest 保存活动 Actor 加载期间暂存的消息和请求信封。
type pendingActivityRequest struct {
	message interface{}
	request *tree.Envelope
}

// Activity 统一管理不同归属对象的活动状态及其存盘生命周期。
type Activity struct {
	serverID      uint64
	router        tree.Router
	messageRouter tree.Router
	wheel         *timer.TimeWheel
	system        *tree.Tree
	selfPID       tree.PID
	store         *mongorpc.Store
	model         *PersistenceModel
	lifecycle     *mongorpc.Lifecycle
	ready         bool
	loadErr       error
	pending       []pendingActivityRequest
	serverOpened  bool
	serverClosing bool
	ownerStates   map[ownerKey]*ownerActivityState
	ownerLoads    int
	shutdownSave  *tree.Envelope
	pendingSaves  []*tree.Envelope
}

// NewActivity 创建可选服务器编号对应的全局活动 Actor。
func NewActivity(serverID ...uint64) *Activity {
	if !RegisterBuiltins() {
		tlog.Log.Error("[activity/NewActivity] 注册内置活动模板生命周期失败")
	}
	currentServerID := defaultActivityServerID
	if len(serverID) > 0 && serverID[0] > 0 {
		currentServerID = serverID[0]
	}
	return &Activity{serverID: currentServerID, ownerStates: make(map[ownerKey]*ownerActivityState)}
}

// Name 返回全局活动 Actor 的名称。
func (actor *Activity) Name() string { return def.ActorActivity }

// OnInit 注册消息路由和存盘模型，并加载服务器活动数据。
func (actor *Activity) OnInit(ctx tree.Context) {
	internal.InitMessageHandlers(
		&actor.messageRouter,
		actor.onServerEvent,
		actor.onSaveAllActivities,
		actor.onPlayerSessionOpened,
		actor.onPlayerSessionClosed,
		actor.onOwnerActivityMessage,
		actor.onPlayerActivityOwnerReleased,
		actor.onLifecycleTick,
		actor.onUnhandledMessage,
	)
	internal.InitRoutes(&actor.router, actor.onGetServerActivityList)
	actor.registerTimers(ctx)
	actor.store = mongorpc.NewStore(def.Mongo)
	actor.lifecycle = mongorpc.NewLifecycle(actor.store, actor.wheel, ctx)
	actor.model = NewPersistenceModel(actor.rangeActivityRecords, actor.afterActivityRecordSaved, activityOwnerSaveInterval)
	if err := activityowner.RegisterLegacyPlayerModel(actor.store); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	if err := actor.lifecycle.Register(actor.model); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	serverKey := ownerKey{scope: activitymodel.ScopeServer, ownerID: actor.serverID}
	if actor.ensureOwnerState(ctx, serverKey) == nil {
		actor.failLoad(ctx, errors.New("server activity state is unavailable"))
		return
	}
	actor.scheduleLifecycleTick()
}

// HandleMessage 将全局活动消息分发给已注册的处理方法。
func (actor *Activity) HandleMessage(ctx tree.Context, message interface{}) {
	actor.messageRouter.Route(ctx, message)
}

// onGetServerActivityList 返回当前服务器归属的活动列表。
func (actor *Activity) onGetServerActivityList(ctx tree.Context, _ *pb_activity.GetActivityListReq) {
	ctx.Response(actor.serverActivityListAck(), nil)
}

// serverActivityListAck 组装当前服务器归属对象的活动列表响应。
func (actor *Activity) serverActivityListAck() *pb_activity.GetActivityListAck {
	key := ownerKey{scope: activitymodel.ScopeServer, ownerID: actor.serverID}
	state := actor.ownerStates[key]
	activities := make([]*pb_activity.ActivityInfo, 0)
	if state != nil && state.ready && state.data != nil && state.data.GetManager() != nil {
		activities = append(activities, activityruntime.Default().GetInfoList(state.data.GetManager())...)
	}
	return &pb_activity.GetActivityListAck{Activities: activities}
}

// onServerEvent 处理服务器开服和关服事件。
func (actor *Activity) onServerEvent(ctx tree.Context, message *sspb.Event) {
	if gameevent.IsServerOpened(message) {
		actor.serverOpened = true
		tlog.Log.Info("[activity/Activity.onServerEvent] 服务器活动 Actor 已处理开服事件")
		ctx.Response(nil, nil)
		return
	}
	if !gameevent.IsServerClosing(message) {
		eventName := message.GetName()
		err := fmt.Errorf("unsupported server activity event %q", eventName)
		tlog.Log.Error("[activity/Activity.onServerEvent] 收到不支持的服务器活动事件, event=%v err=%v", eventName, err)
		ctx.Response(nil, err)
		return
	}
	actor.serverClosing = true
	tlog.Log.Info("[activity/Activity.onServerEvent] 服务器活动 Actor 已接收关服事件")
	if !actor.ready {
		actor.queueRequest(ctx, message)
		return
	}
	if actor.ownerLoads > 0 {
		actor.shutdownSave = ctx.RequestEnvelope()
		return
	}
	actor.saveAll(ctx, ctx.RequestEnvelope())
}

// onSaveAllActivities 处理全局活动数据存盘请求。
func (actor *Activity) onSaveAllActivities(ctx tree.Context, message *ipb.SaveAllActivities) {
	if !actor.ready {
		actor.queueRequest(ctx, message)
		return
	}
	actor.saveAllWhenOwnersReady(ctx, ctx.RequestEnvelope())
}

// onUnhandledMessage 将其他消息排入加载队列或转交活动业务路由。
func (actor *Activity) onUnhandledMessage(ctx tree.Context, message interface{}) {
	if actor.serverClosing {
		ctx.Response(nil, fmt.Errorf("activity actor is shutting down"))
		return
	}
	if actor.loadErr != nil {
		ctx.Response(nil, actor.loadErr)
		return
	}
	if !actor.ready {
		actor.queueRequest(ctx, message)
		return
	}
	actor.router.Route(ctx, message)
}

// OnStop 停止活动存盘生命周期和时间轮。
func (actor *Activity) OnStop(_ tree.Context) {
	if actor.lifecycle != nil {
		actor.lifecycle.Stop()
	}
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
}

// failLoad 记录加载错误并完成已排队请求。
func (actor *Activity) failLoad(ctx tree.Context, err error) {
	actor.loadErr = err
	actor.ready = true
	tlog.Log.Error("[activity/Activity.failLoad] 服务器活动数据加载失败, err=%v", err)
	actor.drainPending(ctx)
}

// queueRequest 在全局活动数据加载期间暂存请求。
func (actor *Activity) queueRequest(ctx tree.Context, message interface{}) {
	request := ctx.RequestEnvelope()
	if request == nil {
		tlog.Log.Error("[activity/Activity.queueRequest] 活动数据加载期间收到无请求信封消息, type=%T", message)
		return
	}
	if len(actor.pending) >= maxPendingActivityRequests {
		request.Respond(nil, fmt.Errorf("server activity request queue is full"))
		tlog.Log.Error("[activity/Activity.queueRequest] 活动加载期间请求队列已满, count=%v", len(actor.pending))
		return
	}
	actor.pending = append(actor.pending, pendingActivityRequest{message: message, request: request})
}

// drainPending 在数据加载完成后按顺序处理暂存请求。
func (actor *Activity) drainPending(ctx tree.Context) {
	pending := actor.pending
	actor.pending = nil
	for _, item := range pending {
		if item.request == nil {
			continue
		}
		if actor.loadErr != nil {
			item.request.Respond(nil, actor.loadErr)
			continue
		}
		if actor.serverClosing {
			if _, isActivityRequest := item.message.(*ipb.PlayerActivityMessage); isActivityRequest {
				item.request.Respond(nil, fmt.Errorf("activity actor is shutting down"))
				continue
			}
		}
		switch message := item.message.(type) {
		case *ipb.SaveAllActivities:
			actor.saveAllWhenOwnersReady(ctx, item.request)
		case *pb_activity.GetActivityListReq:
			item.request.Respond(actor.serverActivityListAck(), nil)
		case *ipb.PlayerActivityMessage:
			actor.dispatchOwnerActivityRequest(ctx, item.request, message)
		default:
			if gameevent.IsServerClosing(message) {
				actor.serverClosing = true
				if actor.ownerLoads > 0 {
					actor.shutdownSave = item.request
				} else {
					actor.saveAll(ctx, item.request)
				}
				continue
			}
			if gameevent.IsServerOpened(message) {
				actor.serverOpened = true
				item.request.Respond(nil, nil)
				continue
			}
			item.request.Respond(nil, fmt.Errorf("unsupported activity request %T", item.message))
		}
	}
}

// saveAllWhenOwnersReady 等待所有归属数据加载完成后执行全量存盘。
func (actor *Activity) saveAllWhenOwnersReady(ctx tree.Context, request *tree.Envelope) {
	if actor.ownerLoads > 0 {
		if request != nil {
			actor.pendingSaves = append(actor.pendingSaves, request)
		}
		return
	}
	actor.saveAll(ctx, request)
}

// onLifecycleTick 推进活动生命周期、处理延迟存盘并安排下一次定时消息。
func (actor *Activity) onLifecycleTick(ctx tree.Context, _ *internal.LifecycleTick) {
	if actor.ready && actor.loadErr == nil && !actor.serverClosing {
		actor.syncOwnerActivities(ctx, time.Now().Unix())
	}
	actor.finishDeferredSaves(ctx)
	actor.scheduleLifecycleTick()
}

// saveAll 存盘当前全局活动 Actor 管理的所有活动记录。
func (actor *Activity) saveAll(ctx tree.Context, request *tree.Envelope) {
	if actor.lifecycle == nil {
		if request != nil {
			request.Respond(nil, fmt.Errorf("activity persistence lifecycle is unavailable"))
		}
		return
	}
	if !actor.ready || actor.loadErr != nil {
		err := actor.loadErr
		if err == nil {
			err = fmt.Errorf("activity data is not ready")
		}
		if request != nil {
			request.Respond(nil, err)
		}
		return
	}
	actor.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[activity/Activity.saveAll] 关服前活动数据存盘失败, err=%v", err)
		} else {
			for key, state := range actor.ownerStates {
				if state != nil && state.releaseRequested && !state.online {
					actor.releaseOwnerState(key, state)
				}
			}
		}
		if request != nil {
			request.Respond(nil, err)
		}
	})
}

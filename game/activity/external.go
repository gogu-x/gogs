package activity

import (
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity/internal"
	"github.com/gogu-x/gogs/game/activity/internal/playeractor"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	activityLifecycleTimerType    = timer.TimerType(1000005)
	activityLifecycleTickInterval = time.Second
	maxPendingActivityRequests    = 1024
)

type pendingActivityRequest struct {
	message interface{}
	request *tree.Envelope
}

// Activity owns server-scoped activity state and its persistence lifecycle.
type Activity struct {
	mgr           *internal.Mgr
	router        tree.Router
	messageRouter tree.Router
	wheel         *timer.TimeWheel
	store         *mongorpc.Store
	model         *PersistenceModel
	lifecycle     *mongorpc.Lifecycle
	ready         bool
	loadErr       error
	pending       []pendingActivityRequest
	serverOpened  bool
	serverClosing bool
}

// NewActivity creates the server activity Actor for an optional server ID.
func NewActivity(serverID ...uint64) *Activity {
	if !RegisterBuiltins() {
		tlog.Log.Error("[activity/NewActivity] 注册内置活动模板生命周期失败")
	}
	return &Activity{mgr: internal.NewMgr(serverID...)}
}

// NewPlayerActivityActor creates an activity Actor independent from PlayerActor.
func NewPlayerActivityActor(uid uint64) tree.Actor {
	return playeractor.New(uid, nil)
}

// NewPlayerActivityActorWithMessages creates an activity Actor with queued offline commands.
func NewPlayerActivityActorWithMessages(uid uint64, messages []interface{}) tree.Actor {
	return playeractor.New(uid, messages)
}

// Name returns the server activity Actor name.
func (actor *Activity) Name() string { return def.ActorActivity }

// OnInit registers routes and persistence before loading server activity data.
func (actor *Activity) OnInit(ctx tree.Context) {
	internal.InitMessageHandlers(
		&actor.messageRouter,
		actor.onServerEvent,
		actor.onSaveAllActivities,
		actor.onUnhandledMessage,
	)
	internal.InitRoutes(&actor.router, actor.mgr)
	actor.wheel = timer.NewTimeWheel(32, ctx.Self(), ctx.System())
	actor.wheel.Register(activityLifecycleTimerType, func(_ interface{}) {
		actor.onLifecycleTick()
	})
	actor.store = mongorpc.NewStore(def.Mongo)
	actor.lifecycle = mongorpc.NewLifecycle(actor.store, actor.wheel, ctx)
	actor.model = NewPersistenceModel(
		func(callback func(*Record) bool) {
			if record := actor.mgr.GetActivityRecord(); record != nil && callback != nil {
				callback(record)
			}
		},
		func(snapshot *Record) {
			current := actor.mgr.GetActivityRecord()
			if current != nil && snapshot != nil && current.GetID() == snapshot.GetID() && current.Revision() == snapshot.Revision() {
				current.SetDirty(false)
			}
		},
		time.Minute,
	)
	if err := actor.lifecycle.Register(actor.model); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	if !actor.lifecycle.LoadOne(
		ctx,
		(*Record)(nil),
		RecordID(ScopeServer, actor.mgr.ServerID()),
		actor.onActivityLoaded,
	) {
		actor.failLoad(ctx, errors.New("server activity load request could not be sent"))
		return
	}
	actor.scheduleLifecycleTick()
}

// HandleMessage routes server activity messages to their registered handlers.
func (actor *Activity) HandleMessage(ctx tree.Context, message interface{}) {
	actor.messageRouter.Route(ctx, message)
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
	actor.saveAll(ctx, ctx.RequestEnvelope())
}

// onSaveAllActivities 处理服务器活动存盘请求。
func (actor *Activity) onSaveAllActivities(ctx tree.Context, message *ipb.SaveAllActivities) {
	if !actor.ready {
		actor.queueRequest(ctx, message)
		return
	}
	actor.saveAll(ctx, ctx.RequestEnvelope())
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

// OnStop stops timers after the caller completes its explicit persistence flush.
func (actor *Activity) OnStop(_ tree.Context) {
	if actor.lifecycle != nil {
		actor.lifecycle.Stop()
	}
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
}

func (actor *Activity) onActivityLoaded(ctx tree.Context, value interface{}, err error) {
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
		value = nil
	}
	if err != nil {
		actor.failLoad(ctx, err)
		return
	}
	if value != nil {
		record, ok := value.(*Record)
		if !ok || record == nil || record.GetScope() != ScopeServer ||
			record.GetOwnerID() != actor.mgr.ServerID() || record.GetID() != RecordID(ScopeServer, actor.mgr.ServerID()) {
			actor.failLoad(ctx, fmt.Errorf("invalid server activity document %T", value))
			return
		}
		if record.GetManager() == nil {
			record.SetManager(NewManager())
			record.MarkDirty()
		}
		actor.mgr.SetActivityRecord(record)
	}
	actor.syncServerActivities(time.Now().Unix(), "onActivityLoaded")
	actor.ready = true
	actor.drainPending(ctx)
}

func (actor *Activity) failLoad(ctx tree.Context, err error) {
	actor.loadErr = err
	actor.ready = true
	tlog.Log.Error("[activity/Activity.failLoad] 服务器活动数据加载失败, err=%v", err)
	actor.drainPending(ctx)
}

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
		switch message := item.message.(type) {
		case *ipb.SaveAllActivities:
			actor.saveAll(ctx, item.request)
		case *pb_activity.GetActivityListReq:
			item.request.Respond(actor.mgr.GetActivityListAck(), nil)
		default:
			if gameevent.IsServerClosing(message) {
				actor.serverClosing = true
				actor.saveAll(ctx, item.request)
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

func (actor *Activity) onLifecycleTick() {
	if actor.ready && actor.loadErr == nil {
		actor.syncServerActivities(time.Now().Unix(), "onLifecycleTick")
	}
	actor.scheduleLifecycleTick()
}

func (actor *Activity) syncServerActivities(now int64, caller string) {
	if actor == nil || actor.mgr == nil {
		return
	}
	changed, transitions := actor.mgr.SyncServerActivities(now)
	if changed {
		tlog.Log.Info("[activity/Activity.%s] 已同步服务器活动模板及生命周期", caller)
	}
	for _, transition := range transitions {
		tlog.Log.Info("[activity/Activity.%s] 服务器活动状态已变更, activity=%v from=%v to=%v owner=%v",
			caller, transition.GetTemplateID(), transition.GetFrom(), transition.GetTo(), transition.GetOwnerID())
	}
}

func (actor *Activity) scheduleLifecycleTick() {
	if actor.wheel == nil {
		return
	}
	if actor.wheel.After(activityLifecycleTimerType, activityLifecycleTickInterval, nil) == nil {
		tlog.Log.Error("[activity/Activity.scheduleLifecycleTick] 安排服务器活动定时器失败")
	}
}

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
			tlog.Log.Error("[activity/Activity.saveAll] 关服前服务器活动存盘失败, err=%v", err)
		}
		if request != nil {
			request.Respond(nil, err)
		}
	})
}

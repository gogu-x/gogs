// Package playeractor runs player-owned activity state independently from PlayerActor.
package playeractor

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gogu-x/gogs/def"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityowner "github.com/gogu-x/gogs/game/activity/internal/owner"
	activityprotocol "github.com/gogu-x/gogs/game/activity/internal/protocol"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
)

const (
	playerActivityTickType     = timer.TimerType(1000008)
	playerActivityTickInterval = time.Second
	maxPendingMessages         = 512
)

// Actor owns one player's activities and their separate persistence record.
type Actor struct {
	UID  uint64       // 玩家唯一 ID。
	Boot func(*Actor) // Boot 在路由与加载初始化阶段调用。

	initialMessages    []interface{}
	owner              *activityowner.Data
	store              *mongorpc.Store
	lifecycle          *mongorpc.Lifecycle
	wheel              *timer.TimeWheel
	router             tree.Router
	messageRouter      tree.Router
	gatePID            tree.PID
	ready              bool
	loadErr            error
	pending            []interface{}
	serverCloseRequest *tree.Envelope
	sessionActive      bool
	sessionGen         uint64
	serverOpened       bool
	serverClosing      bool
}

// New creates a player activity Actor.
func New(uid uint64, initialMessages []interface{}) *Actor {
	return &Actor{UID: uid, initialMessages: append([]interface{}(nil), initialMessages...)}
}

// Name returns this player's activity Actor name.
func (actor *Actor) Name() string { return fmt.Sprintf("activity-player-%d", actor.UID) }

// MailboxSize gives each player activity Actor an independent bounded mailbox.
func (actor *Actor) MailboxSize() int { return 512 }

// OnInit registers activity persistence and starts loading this player's activity data.
func (actor *Actor) OnInit(ctx tree.Context) {
	actor.registerMessageHandlers()
	actor.registerRoutes()
	actor.gatePID, _ = ctx.Lookup(def.GameGate)
	actor.owner = activityowner.New(activitymodel.ScopePlayer, actor.UID)
	actor.store = mongorpc.NewStore(def.Mongo)
	if err := actor.owner.Register(actor.store); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	actor.wheel = timer.NewTimeWheel(32, ctx.Self(), ctx.System())
	actor.wheel.Register(playerActivityTickType, func(_ interface{}) {
		actor.onActivityTick()
	})
	actor.lifecycle = mongorpc.NewLifecycle(actor.store, actor.wheel, ctx)
	if err := actor.lifecycle.Register(actor.owner.PersistenceModel()); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	for _, message := range actor.initialMessages {
		actor.queue(message)
	}
	if actor.Boot != nil {
		actor.Boot(actor)
	}
	if actor.UID == 0 {
		actor.failLoad(ctx, errors.New("player activity actor requires a non-zero uid"))
		return
	}
	if !actor.owner.Load(ctx, actor.store, actor.onOwnerLoaded) {
		actor.failLoad(ctx, errors.New("player activity load request could not be sent"))
		return
	}
	actor.scheduleActivityTick()
}

// OnStop stops lifecycle timers after the activity state is flushed.
func (actor *Actor) OnStop(_ tree.Context) {
	if actor.lifecycle != nil {
		actor.lifecycle.Stop()
	}
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
}

// MarkDirty marks this owner's activity record after a business mutation.
func (actor *Actor) MarkDirty() {
	if actor != nil && actor.owner != nil {
		actor.owner.MarkDirty()
	}
}

func (actor *Actor) onOwnerLoaded(ctx tree.Context, err error) {
	if err != nil {
		actor.failLoad(ctx, err)
		return
	}
	transitions, changed := actor.owner.Sync(time.Now().Unix())
	actor.logTransitions(transitions)
	if changed {
		tlog.Log.Info("[activity/playeractor.onOwnerLoaded] 玩家活动数据已同步, activities=%v pid=%v",
			len(actor.owner.GetManager().List()), actor.UID)
	}
	actor.ready = true
	actor.drainPending(ctx)
	actor.savePendingServerClose(ctx)
}

func (actor *Actor) failLoad(ctx tree.Context, err error) {
	actor.loadErr = err
	actor.ready = true
	tlog.Log.Error("[activity/playeractor.failLoad] 玩家活动数据加载失败, pid=%v err=%v", actor.UID, err)
	actor.drainPending(ctx)
	if actor.serverCloseRequest != nil {
		request := actor.serverCloseRequest
		actor.serverCloseRequest = nil
		request.Respond(nil, err)
	}
}

// onSessionOpened 绑定属于当前玩家 Actor 的新会话。
func (actor *Actor) onSessionOpened(_ tree.Context, message *ipb.PlayerSessionOpened) {
	if message == nil {
		return
	}
	uid := message.GetUID()
	generation := message.GetGeneration()
	if uid != actor.UID || generation <= actor.sessionGen {
		return
	}
	actor.sessionGen = generation
	actor.sessionActive = true
	tlog.Log.Info("[activity/playeractor.onSessionOpened] 玩家活动会话已绑定, pid=%v generation=%v", actor.UID, actor.sessionGen)
}

func (actor *Actor) onSessionClosed(ctx tree.Context, message *ipb.SessionClosed) {
	if message == nil {
		return
	}
	uid := message.GetUID()
	generation := message.GetGeneration()
	if uid != actor.UID || generation != actor.sessionGen || !actor.sessionActive {
		return
	}
	actor.sessionActive = false
	if actor.ready && actor.loadErr == nil {
		transitions, _ := actor.owner.Sync(time.Now().Unix())
		actor.logTransitions(transitions)
		actor.saveAll(ctx, nil)
	}
}

func (actor *Actor) onPlayerInbound(ctx tree.Context, message *ipb.PlayerInbound) {
	if message == nil {
		return
	}
	uid := message.GetUID()
	generation := message.GetGeneration()
	if uid != actor.UID || generation != actor.sessionGen || !actor.sessionActive {
		return
	}
	actor.routeOrQueue(ctx, message.GetMessage())
}

func (actor *Actor) routeOrQueue(ctx tree.Context, message interface{}) {
	if actor.serverClosing {
		actor.pushError(ctx, message, "player activity actor is shutting down")
		return
	}
	if actor.loadErr != nil {
		actor.pushError(ctx, message, actor.loadErr.Error())
		return
	}
	if !actor.ready {
		actor.queue(message)
		return
	}
	actor.router.Route(ctx, message)
}

func (actor *Actor) queue(message interface{}) {
	if message == nil {
		return
	}
	if len(actor.pending) >= maxPendingMessages {
		tlog.Log.Error("[activity/playeractor.queue] 玩家活动加载期间请求队列已满, pid=%v count=%v", actor.UID, len(actor.pending))
		return
	}
	actor.pending = append(actor.pending, message)
}

func (actor *Actor) drainPending(ctx tree.Context) {
	pending := actor.pending
	actor.pending = nil
	for _, message := range pending {
		if actor.loadErr != nil {
			actor.pushError(ctx, message, actor.loadErr.Error())
			continue
		}
		if gameevent.IsServerOpened(message) {
			actor.serverOpened = true
			continue
		}
		if gameevent.IsServerClosing(message) {
			actor.serverClosing = true
			continue
		}
		actor.router.Route(ctx, message)
	}
}

func (actor *Actor) savePendingServerClose(ctx tree.Context) {
	if actor.serverCloseRequest == nil {
		return
	}
	request := actor.serverCloseRequest
	actor.serverCloseRequest = nil
	actor.saveAll(ctx, request)
}

func (actor *Actor) onActivityTick() {
	if actor.ready && actor.loadErr == nil {
		transitions, changed := actor.owner.Sync(time.Now().Unix())
		actor.logTransitions(transitions)
		if changed {
			tlog.Log.Info("[activity/playeractor.onActivityTick] 玩家活动生命周期已推进, pid=%v", actor.UID)
		}
	}
	actor.scheduleActivityTick()
}

func (actor *Actor) scheduleActivityTick() {
	if actor.wheel == nil {
		return
	}
	if actor.wheel.After(playerActivityTickType, playerActivityTickInterval, nil) == nil {
		tlog.Log.Error("[activity/playeractor.scheduleActivityTick] 安排玩家活动定时器失败, pid=%v", actor.UID)
	}
}

func (actor *Actor) saveAll(ctx tree.Context, request *tree.Envelope) {
	if actor.lifecycle == nil || actor.loadErr != nil || !actor.ready {
		err := actor.loadErr
		if err == nil {
			err = errors.New("player activity persistence is unavailable")
		}
		if request != nil {
			request.Respond(nil, err)
		}
		return
	}
	actor.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[activity/playeractor.saveAll] 玩家活动存盘失败, pid=%v err=%v", actor.UID, err)
		}
		if request != nil {
			request.Respond(nil, err)
		}
	})
}

func (actor *Actor) getActivityList(ctx tree.Context, request *pb_activity.GetActivityListReq) {
	if !actor.validUID(request.GetUID()) {
		return
	}
	transitions, changed := activityruntime.Default().Advance(actor.owner.GetManager(), time.Now().Unix())
	if changed {
		actor.MarkDirty()
	}
	actor.logTransitions(transitions)
	activities := make([]*pb_activity.ActivityInfo, 0)
	for _, instance := range actor.owner.GetManager().List() {
		if instance != nil {
			activities = append(activities, activityprotocol.ToInfo(instance))
		}
	}
	actor.requestServerActivities(ctx, activities)
}

func (actor *Actor) requestServerActivities(ctx tree.Context, activities []*pb_activity.ActivityInfo) {
	serverPID, ok := ctx.Lookup(def.ActorActivity)
	if !ok {
		actor.requestGuildActivities(ctx, activities)
		return
	}
	request := ctx.RequestCallback(serverPID, &pb_activity.GetActivityListReq{UID: actor.UID}, func(callbackCtx tree.Context, value interface{}, err error) {
		if err == nil {
			if serverAck, valid := value.(*pb_activity.GetActivityListAck); valid && serverAck != nil {
				activities = append(activities, serverAck.GetActivities()...)
			} else {
				tlog.Log.Error("[activity/playeractor.requestServerActivities] 服务器活动响应类型无效, pid=%v type=%T", actor.UID, value)
			}
		} else {
			tlog.Log.Error("[activity/playeractor.requestServerActivities] 获取服务器活动失败, pid=%v err=%v", actor.UID, err)
		}
		actor.requestGuildActivities(callbackCtx, activities)
	})
	if request == nil {
		actor.requestGuildActivities(ctx, activities)
	}
}

func (actor *Actor) requestGuildActivities(ctx tree.Context, activities []*pb_activity.ActivityInfo) {
	guildPID, ok := ctx.Lookup(def.Guild)
	if !ok {
		actor.push(ctx, &pb_activity.GetActivityListAck{Activities: sortActivities(activities)})
		return
	}
	request := ctx.RequestCallback(guildPID, &ipb.GetGuildActivities{UID: actor.UID}, func(callbackCtx tree.Context, value interface{}, err error) {
		if err == nil {
			if guildActivities, valid := value.([]*pb_activity.ActivityInfo); valid {
				activities = append(activities, guildActivities...)
			} else {
				tlog.Log.Error("[activity/playeractor.requestGuildActivities] 联盟活动响应类型无效, pid=%v type=%T", actor.UID, value)
			}
		} else {
			tlog.Log.Error("[activity/playeractor.requestGuildActivities] 获取联盟活动失败, pid=%v err=%v", actor.UID, err)
		}
		actor.push(callbackCtx, &pb_activity.GetActivityListAck{Activities: sortActivities(activities)})
	})
	if request == nil {
		actor.push(ctx, &pb_activity.GetActivityListAck{Activities: sortActivities(activities)})
	}
}

func (actor *Actor) joinActivity(ctx tree.Context, request *pb_activity.JoinActivityReq) {
	if !actor.validUID(request.GetUID()) {
		return
	}
	manager := actor.owner.GetManager()
	activityID := request.GetActivityId()
	instance, exists := manager.Get(activityID)
	transitions := make([]activitymodel.Transition, 0)
	if !exists {
		template, found := activitymodel.DefaultCatalog().Get(activityID)
		if !found || template.GetScope() != activitymodel.ScopePlayer {
			actor.push(ctx, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "player activity is not available"})
			return
		}
		instance, transitions, exists = activityruntime.Default().Create(manager, template, actor.UID, time.Now().Unix())
		if !exists {
			actor.push(ctx, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "player activity could not be created"})
			return
		}
		actor.MarkDirty()
	} else {
		advanced, changed := activityruntime.Default().Advance(manager, time.Now().Unix())
		transitions = append(transitions, advanced...)
		if changed {
			actor.MarkDirty()
		}
	}
	actor.logTransitions(transitions)
	if instance.GetStatus() != activitymodel.StatusRunning {
		actor.push(ctx, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "activity is not running"})
		return
	}
	tlog.Log.Info("[activity/playeractor.joinActivity] 玩家已加入活动, activity=%v pid=%v", activityID, actor.UID)
	actor.push(ctx, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_OK})
}

func (actor *Actor) getProgress(ctx tree.Context, request *pb_activity.GetProgressReq) {
	if !actor.validUID(request.GetUID()) {
		return
	}
	manager := actor.owner.GetManager()
	instance, exists := manager.Get(request.GetActivityId())
	if !exists || instance == nil || instance.GetScope() != activitymodel.ScopePlayer || instance.GetOwnerID() != actor.UID {
		actor.push(ctx, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_UNKNOWN})
		return
	}
	transitions, changed := activityruntime.Default().Advance(manager, time.Now().Unix())
	if changed {
		actor.MarkDirty()
	}
	actor.logTransitions(transitions)
	actor.push(ctx, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_OK, Progress: activityprotocol.ToProgress(instance)})
}

func (actor *Actor) claimReward(ctx tree.Context, request *pb_activity.ClaimRewardReq) {
	if !actor.validUID(request.GetUID()) {
		return
	}
	manager := actor.owner.GetManager()
	transitions, changed := activityruntime.Default().Advance(manager, time.Now().Unix())
	if changed {
		actor.MarkDirty()
	}
	actor.logTransitions(transitions)
	activityID := request.GetActivityId()
	if !activityruntime.Default().Claim(manager, activityID, time.Now().Unix()) {
		actor.push(ctx, &pb_activity.ClaimRewardAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "activity is incomplete or reward was already claimed"})
		return
	}
	actor.MarkDirty()
	tlog.Log.Info("[activity/playeractor.claimReward] 玩家已领取活动奖励, activity=%v pid=%v", activityID, actor.UID)
	actor.push(ctx, &pb_activity.ClaimRewardAck{Code: pb_common.ErrCode_OK})
}

func (actor *Actor) validUID(uid uint64) bool {
	if uid == actor.UID {
		return true
	}
	tlog.Log.Error("[activity/playeractor.validUID] 玩家活动请求 UID 与 Actor 不匹配, pid=%v requestUID=%v", actor.UID, uid)
	return false
}

func (actor *Actor) push(ctx tree.Context, message interface{}) bool {
	gatePID := actor.gatePID
	if gatePID.ID == 0 {
		gatePID, _ = ctx.Lookup(def.GameGate)
	}
	if gatePID.ID == 0 {
		return false
	}
	return ctx.Send(gatePID, &ipb.PushToMsg{UID: actor.UID, Msg: message})
}

func (actor *Actor) pushError(ctx tree.Context, message interface{}, reason string) {
	switch message.(type) {
	case *pb_activity.GetActivityListReq:
		actor.push(ctx, &pb_activity.GetActivityListAck{Activities: []*pb_activity.ActivityInfo{}})
	case *pb_activity.JoinActivityReq:
		actor.push(ctx, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_INTERNAL, Msg: reason})
	case *pb_activity.GetProgressReq:
		actor.push(ctx, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_INTERNAL})
	case *pb_activity.ClaimRewardReq:
		actor.push(ctx, &pb_activity.ClaimRewardAck{Code: pb_common.ErrCode_INTERNAL, Msg: reason})
	}
}

func (actor *Actor) logTransitions(transitions []activitymodel.Transition) {
	for _, transition := range transitions {
		tlog.Log.Info("[activity/playeractor.lifecycle] 玩家活动状态已变更, activity=%v from=%v to=%v pid=%v",
			transition.GetTemplateID(), transition.GetFrom(), transition.GetTo(), actor.UID)
	}
}

func sortActivities(activities []*pb_activity.ActivityInfo) []*pb_activity.ActivityInfo {
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].GetId() < activities[j].GetId()
	})
	return activities
}

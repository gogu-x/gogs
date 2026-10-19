package activity

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gogu-x/gogs/def"
	activitymodel "github.com/gogu-x/gogs/game/activity/internal/model"
	activityowner "github.com/gogu-x/gogs/game/activity/internal/owner"
	activityruntime "github.com/gogu-x/gogs/game/activity/internal/runtime"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	// maxPendingOwnerActivityRequests 表示单个归属对象加载期间可暂存的请求上限。
	maxPendingOwnerActivityRequests = 512
)

// ownerKey 标识一份活动数据所属的聚合对象。
type ownerKey struct {
	scope   activitymodel.OwnerScope
	ownerID uint64
}

// ownerActivityState 保存一个归属对象在全局活动 Actor 中的运行状态。
type ownerActivityState struct {
	data             *activityowner.Data
	pending          []pendingActivityRequest
	generation       uint64
	ready            bool
	loading          bool
	online           bool
	saving           bool
	saveRequested    bool
	releaseRequested bool
}

// onPlayerSessionOpened 在玩家登录时加载并绑定该玩家所属的活动数据。
func (actor *Activity) onPlayerSessionOpened(ctx tree.Context, message *ipb.PlayerSessionOpened) {
	if message == nil || message.GetUID() == 0 || actor.serverClosing || actor.loadErr != nil {
		return
	}
	uid := message.GetUID()
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ensureOwnerState(ctx, key)
	if state == nil || message.GetGeneration() < state.generation {
		return
	}
	state.generation = message.GetGeneration()
	state.online = true
	state.releaseRequested = false
}

// onPlayerSessionClosed 标记玩家离线并立即存盘其归属活动数据。
func (actor *Activity) onPlayerSessionClosed(ctx tree.Context, message *ipb.SessionClosed) {
	if message == nil || message.GetUID() == 0 {
		return
	}
	uid := message.GetUID()
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ownerStates[key]
	if state == nil || message.GetGeneration() != state.generation || !state.online {
		return
	}
	state.online = false
	state.saveRequested = true
	actor.saveOwnerState(ctx, key, state)
}

// onOwnerActivityMessage 接收玩家 Actor 转发的活动业务请求。
func (actor *Activity) onOwnerActivityMessage(ctx tree.Context, message *ipb.PlayerActivityMessage) {
	if message == nil || message.GetUID() == 0 || message.GetMessage() == nil {
		ctx.Response(nil, fmt.Errorf("invalid activity owner request"))
		return
	}
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
	actor.dispatchOwnerActivityRequest(ctx, ctx.RequestEnvelope(), message)
}

// onPlayerActivityOwnerReleased 在玩家 Actor 回收时保存并释放该玩家的归属活动数据。
func (actor *Activity) onPlayerActivityOwnerReleased(ctx tree.Context, message *ipb.PlayerActivityOwnerReleased) {
	if message == nil || message.GetUID() == 0 {
		return
	}
	uid := message.GetUID()
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ownerStates[key]
	if state == nil || state.online {
		return
	}
	state.releaseRequested = true
	state.saveRequested = true
	actor.saveOwnerState(ctx, key, state)
}

// ensureOwnerState 创建归属对象的活动状态并异步加载独立活动记录。
func (actor *Activity) ensureOwnerState(ctx tree.Context, key ownerKey) *ownerActivityState {
	if key.ownerID == 0 || actor.ownerStates == nil {
		return nil
	}
	if state := actor.ownerStates[key]; state != nil {
		return state
	}
	state := &ownerActivityState{
		data:    activityowner.New(key.scope, key.ownerID),
		loading: true,
	}
	actor.ownerStates[key] = state
	actor.ownerLoads++
	if !state.data.Load(ctx, actor.store, func(callbackCtx tree.Context, err error) {
		actor.onOwnerActivityLoaded(callbackCtx, key, state, err)
	}) {
		actor.onOwnerActivityLoaded(ctx, key, state, fmt.Errorf("activity owner load request could not be sent"))
	}
	return state
}

// onOwnerActivityLoaded 完成归属数据加载、模板同步及排队请求处理。
func (actor *Activity) onOwnerActivityLoaded(
	ctx tree.Context,
	key ownerKey,
	state *ownerActivityState,
	err error,
) {
	if actor.ownerLoads > 0 {
		actor.ownerLoads--
	}
	if state == nil || actor.ownerStates[key] != state {
		actor.finishDeferredSaves(ctx)
		return
	}
	state.loading = false
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
	}
	if err != nil {
		if key.scope == activitymodel.ScopeServer {
			actor.failLoad(ctx, err)
			return
		}
		tlog.Log.Error("[activity/Activity.onOwnerActivityLoaded] 归属对象活动数据加载失败, scope=%v owner=%v pid=%v err=%v",
			key.scope, key.ownerID, ownerPlayerID(key), err)
		for _, pending := range state.pending {
			if pending.request != nil {
				pending.request.Respond(nil, err)
			}
		}
		delete(actor.ownerStates, key)
		actor.finishDeferredSaves(ctx)
		return
	}
	state.ready = true
	transitions, changed := state.data.Sync(time.Now().Unix())
	actor.logOwnerTransitions(key, transitions)
	if changed {
		tlog.Log.Info("[activity/Activity.onOwnerActivityLoaded] 活动模板已同步, scope=%v owner=%v activities=%v pid=%v",
			key.scope, key.ownerID, len(state.data.GetManager().List()), ownerPlayerID(key))
	}
	if key.scope == activitymodel.ScopeServer {
		actor.ready = true
		actor.drainPending(ctx)
	}
	pending := state.pending
	state.pending = nil
	for _, request := range pending {
		actor.processOwnerActivityRequest(ctx, key.ownerID, state, request)
	}
	if state.saveRequested {
		actor.saveOwnerState(ctx, key, state)
	}
	actor.finishDeferredSaves(ctx)
}

// dispatchOwnerActivityRequest 校验请求归属并排队或执行活动请求。
func (actor *Activity) dispatchOwnerActivityRequest(
	ctx tree.Context,
	request *tree.Envelope,
	message *ipb.PlayerActivityMessage,
) {
	uid := message.GetUID()
	if uid == 0 || message.GetMessage() == nil {
		if request != nil {
			request.Respond(nil, fmt.Errorf("invalid activity owner request"))
		}
		return
	}
	requestUID, hasUID := message.GetMessage().(interface{ GetUID() uint64 })
	if !hasUID || requestUID.GetUID() != uid {
		if request != nil {
			request.Respond(nil, fmt.Errorf("activity owner request uid mismatch"))
		}
		return
	}
	if message.GetOnline() && message.GetGeneration() == 0 {
		if request != nil {
			request.Respond(nil, fmt.Errorf("online activity owner request has no session generation"))
		}
		return
	}
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ensureOwnerState(ctx, key)
	if state == nil {
		if request != nil {
			request.Respond(nil, fmt.Errorf("activity owner state is unavailable"))
		}
		return
	}
	if message.GetOnline() {
		generation := message.GetGeneration()
		if state.generation > generation {
			if request != nil {
				request.Respond(nil, fmt.Errorf("stale activity owner session"))
			}
			return
		}
		state.generation = generation
		state.online = true
		state.releaseRequested = false
	}
	pending := pendingActivityRequest{message: message, request: request}
	if state.loading {
		if len(state.pending) >= maxPendingOwnerActivityRequests {
			if request != nil {
				request.Respond(nil, fmt.Errorf("activity owner request queue is full"))
			}
			return
		}
		state.pending = append(state.pending, pending)
		return
	}
	actor.processOwnerActivityRequest(ctx, uid, state, pending)
}

// processOwnerActivityRequest 根据协议请求执行相应的活动模板操作。
func (actor *Activity) processOwnerActivityRequest(
	ctx tree.Context,
	uid uint64,
	state *ownerActivityState,
	pending pendingActivityRequest,
) {
	message, ok := pending.message.(*ipb.PlayerActivityMessage)
	if !ok || message == nil {
		if pending.request != nil {
			pending.request.Respond(nil, fmt.Errorf("invalid activity owner message %T", pending.message))
		}
		return
	}
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	transitions, changed := state.data.Sync(time.Now().Unix())
	actor.logOwnerTransitions(key, transitions)
	if changed {
		tlog.Log.Info("[activity/Activity.processOwnerActivityRequest] 活动数据已推进, pid=%v type=%T", uid, message.GetMessage())
	}
	switch request := message.GetMessage().(type) {
	case *pb_activity.GetActivityListReq:
		actor.getActivityList(ctx, uid, pending.request)
	case *pb_activity.JoinActivityReq:
		actor.joinActivity(uid, pending.request, request)
	case *pb_activity.GetProgressReq:
		actor.getActivityProgress(uid, pending.request, request)
	case *pb_activity.ClaimRewardReq:
		actor.claimActivityReward(uid, pending.request, request)
	default:
		if pending.request != nil {
			pending.request.Respond(nil, fmt.Errorf("unsupported activity request %T", message.GetMessage()))
		}
	}
}

// getActivityList 合并当前玩家、服务器和联盟活动后回复列表。
func (actor *Activity) getActivityList(ctx tree.Context, uid uint64, request *tree.Envelope) {
	activities := make([]*pb_activity.ActivityInfo, 0)
	keys := []ownerKey{
		{scope: activitymodel.ScopePlayer, ownerID: uid},
		{scope: activitymodel.ScopeServer, ownerID: actor.serverID},
	}
	for _, key := range keys {
		state := actor.ownerStates[key]
		if state == nil || !state.ready || state.data == nil || state.data.GetManager() == nil {
			continue
		}
		activities = append(activities, activityruntime.Default().GetInfoList(state.data.GetManager())...)
	}
	guildPID, ok := ctx.Lookup(def.Guild)
	if !ok {
		actor.respondActivityList(request, activities)
		return
	}
	callback := func(callbackCtx tree.Context, value interface{}, err error) {
		if err == nil {
			if guildActivities, valid := value.([]*pb_activity.ActivityInfo); valid {
				activities = append(activities, guildActivities...)
			} else {
				tlog.Log.Error("[activity/Activity.getActivityList] 联盟活动响应类型无效, pid=%v type=%T", uid, value)
			}
		} else {
			tlog.Log.Error("[activity/Activity.getActivityList] 获取联盟活动失败, pid=%v err=%v", uid, err)
		}
		actor.respondActivityList(request, activities)
	}
	if ctx.RequestCallback(guildPID, &ipb.GetGuildActivities{UID: uid}, callback) == nil {
		actor.respondActivityList(request, activities)
	}
}

// respondActivityList 按活动编号排序并回复活动列表。
func (actor *Activity) respondActivityList(request *tree.Envelope, activities []*pb_activity.ActivityInfo) {
	if request == nil {
		return
	}
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].GetId() < activities[j].GetId()
	})
	request.Respond(&pb_activity.GetActivityListAck{Activities: activities}, nil)
}

// joinActivity 按活动实例 ID 查找实例并执行其类型绑定的加入行为。
func (actor *Activity) joinActivity(uid uint64, requestEnvelope *tree.Envelope, request *pb_activity.JoinActivityReq) {
	instanceID := request.GetActivityId()
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ownerStates[key]
	if state == nil || !state.ready || state.data == nil || state.data.GetManager() == nil {
		actor.respondActivityError(requestEnvelope, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "player activity data is not available"})
		return
	}
	manager := state.data.GetManager()
	_, _, instance, found := actor.findActivityForRequest(uid, instanceID)
	if !found || !activityruntime.Default().Join(manager, instanceID, time.Now().Unix()) {
		actor.respondActivityError(requestEnvelope, &pb_activity.JoinActivityAck{Code: pb_common.ErrCode_UNKNOWN, Msg: "activity is not available"})
		return
	}
	tlog.Log.Info("[activity/Activity.joinActivity] 玩家已加入活动, activity=%v scope=%v owner=%v pid=%v",
		instance.GetID(), key.scope, key.ownerID, uid)
	if requestEnvelope != nil {
		requestEnvelope.Respond(&pb_activity.JoinActivityAck{Code: pb_common.ErrCode_OK}, nil)
	}
}

// getActivityProgress 返回玩家可访问的活动实例当前进度。
func (actor *Activity) getActivityProgress(uid uint64, requestEnvelope *tree.Envelope, request *pb_activity.GetProgressReq) {
	key, state, instance, found := actor.findActivityForRequest(uid, request.GetActivityId())
	if !found || state == nil || instance == nil || instance.GetScope() != key.scope || instance.GetOwnerID() != key.ownerID {
		actor.respondActivityError(requestEnvelope, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_UNKNOWN})
		return
	}
	progress, found := activityruntime.Default().GetProgress(state.data.GetManager(), instance.GetID())
	if !found || progress == nil {
		actor.respondActivityError(requestEnvelope, &pb_activity.GetProgressAck{Code: pb_common.ErrCode_UNKNOWN})
		return
	}
	if requestEnvelope != nil {
		requestEnvelope.Respond(&pb_activity.GetProgressAck{
			Code:     pb_common.ErrCode_OK,
			Progress: progress,
		}, nil)
	}
}

// claimActivityReward 按模板生命周期规则领取活动奖励并返回处理结果。
func (actor *Activity) claimActivityReward(uid uint64, requestEnvelope *tree.Envelope, request *pb_activity.ClaimRewardReq) {
	instanceID := request.GetActivityId()
	key, state, _, found := actor.findActivityForRequest(uid, instanceID)
	if !found || state == nil || state.data == nil || !activityruntime.Default().ClaimReward(state.data.GetManager(), instanceID, time.Now().Unix()) {
		actor.respondActivityError(requestEnvelope, &pb_activity.ClaimRewardAck{
			Code: pb_common.ErrCode_UNKNOWN,
			Msg:  "activity is incomplete or reward was already claimed",
		})
		return
	}
	state.data.MarkDirty()
	tlog.Log.Info("[activity/Activity.claimActivityReward] 玩家已领取活动奖励, activity=%v scope=%v owner=%v pid=%v",
		instanceID, key.scope, key.ownerID, uid)
	if requestEnvelope != nil {
		requestEnvelope.Respond(&pb_activity.ClaimRewardAck{Code: pb_common.ErrCode_OK}, nil)
	}
}

// findActivityForRequest 按活动实例 ID 定位当前玩家归属的实例。
func (actor *Activity) findActivityForRequest(
	uid uint64,
	instanceID uint64,
) (ownerKey, *ownerActivityState, *activitymodel.Instance, bool) {
	key := ownerKey{scope: activitymodel.ScopePlayer, ownerID: uid}
	state := actor.ownerStates[key]
	if state == nil || !state.ready || state.data == nil || state.data.GetManager() == nil {
		return key, state, nil, false
	}
	instance, exists := state.data.GetManager().Get(instanceID)
	if !exists || instance == nil || instance.GetScope() != key.scope || instance.GetOwnerID() != uid {
		return key, state, nil, false
	}
	return key, state, instance, true
}

// respondActivityError 返回活动业务错误响应。
func (actor *Activity) respondActivityError(request *tree.Envelope, response interface{}) {
	if request != nil {
		request.Respond(response, nil)
	}
}

// syncOwnerActivities 推进所有已加载归属对象的活动生命周期。
func (actor *Activity) syncOwnerActivities(ctx tree.Context, now int64) {
	keys := make([]ownerKey, 0, len(actor.ownerStates))
	for key := range actor.ownerStates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		return keys[i].ownerID < keys[j].ownerID
	})
	for _, key := range keys {
		state := actor.ownerStates[key]
		if state == nil || !state.ready || state.data == nil {
			continue
		}
		transitions, changed := state.data.Sync(now)
		actor.logOwnerTransitions(key, transitions)
		if changed {
			tlog.Log.Info("[activity/Activity.syncOwnerActivities] 归属对象活动生命周期已推进, scope=%v owner=%v pid=%v",
				key.scope, key.ownerID, ownerPlayerID(key))
		}
		if state.saveRequested {
			actor.saveOwnerState(ctx, key, state)
		}
	}
}

// logOwnerTransitions 记录归属对象活动生命周期状态变化。
func (actor *Activity) logOwnerTransitions(key ownerKey, transitions []activitymodel.Transition) {
	for _, transition := range transitions {
		tlog.Log.Info("[activity/Activity.logOwnerTransitions] 活动状态已变更, instance=%v template=%v from=%v to=%v scope=%v owner=%v pid=%v",
			transition.GetInstanceID(), transition.GetTemplateID(), transition.GetFrom(), transition.GetTo(), key.scope, key.ownerID, ownerPlayerID(key))
	}
}

// ownerPlayerID 在归属范围为玩家时返回玩家编号，其他范围返回零。
func ownerPlayerID(key ownerKey) uint64 {
	if key.scope == activitymodel.ScopePlayer {
		return key.ownerID
	}
	return 0
}

// rangeActivityRecords 按稳定顺序遍历所有已加载的活动存盘记录。
func (actor *Activity) rangeActivityRecords(callback func(*Record) bool) {
	if actor == nil || callback == nil {
		return
	}
	keys := make([]ownerKey, 0, len(actor.ownerStates))
	for key := range actor.ownerStates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		return keys[i].ownerID < keys[j].ownerID
	})
	for _, key := range keys {
		state := actor.ownerStates[key]
		if state == nil || state.loading || state.data == nil {
			continue
		}
		if record := state.data.GetRecord(); record != nil && !callback(record) {
			return
		}
	}
}

// afterActivityRecordSaved 仅在活动记录版本未变化时清除其脏标记。
func (actor *Activity) afterActivityRecordSaved(snapshot *Record) {
	if actor == nil || snapshot == nil {
		return
	}
	key := ownerKey{scope: snapshot.GetScope(), ownerID: snapshot.GetOwnerID()}
	state := actor.ownerStates[key]
	if state == nil || state.data == nil {
		return
	}
	current := state.data.GetRecord()
	if current != nil && current.GetID() == snapshot.GetID() && current.Revision() == snapshot.Revision() {
		current.SetDirty(false)
	}
}

// saveOwnerState 保存归属对象的脏活动记录，并在收到释放请求时回收内存状态。
func (actor *Activity) saveOwnerState(ctx tree.Context, key ownerKey, state *ownerActivityState) {
	if state == nil || state.data == nil {
		return
	}
	if state.loading {
		state.saveRequested = true
		return
	}
	if actor.serverClosing {
		state.saveRequested = true
		return
	}
	if state.saving {
		state.saveRequested = true
		return
	}
	record := state.data.GetRecord()
	if record == nil || !record.IsDirty() {
		state.saveRequested = false
		actor.releaseOwnerState(key, state)
		return
	}
	state.saving = true
	state.saveRequested = false
	if !actor.lifecycle.Save(ctx, (*Record)(nil), record, func(callbackCtx tree.Context, err error) {
		state.saving = false
		if err != nil {
			tlog.Log.Error("[activity/Activity.saveOwnerState] 活动数据存盘失败, scope=%v owner=%v pid=%v err=%v",
				key.scope, key.ownerID, ownerPlayerID(key), err)
			state.saveRequested = true
			return
		}
		tlog.Log.Info("[activity/Activity.saveOwnerState] 活动数据已存盘, scope=%v owner=%v pid=%v",
			key.scope, key.ownerID, ownerPlayerID(key))
		if current := state.data.GetRecord(); current != nil && current.IsDirty() {
			actor.saveOwnerState(callbackCtx, key, state)
			return
		}
		state.saveRequested = false
		actor.releaseOwnerState(key, state)
		actor.finishDeferredSaves(callbackCtx)
	}) {
		state.saving = false
		state.saveRequested = true
		tlog.Log.Error("[activity/Activity.saveOwnerState] 活动数据存盘请求未发送, scope=%v owner=%v pid=%v",
			key.scope, key.ownerID, ownerPlayerID(key))
	}
}

// releaseOwnerState 在归属对象离线且收到释放请求后移除其活动数据。
func (actor *Activity) releaseOwnerState(key ownerKey, state *ownerActivityState) {
	if state != nil && state.releaseRequested && !state.online && actor.ownerStates[key] == state {
		delete(actor.ownerStates, key)
		tlog.Log.Info("[activity/Activity.releaseOwnerState] 归属对象活动数据已释放, scope=%v owner=%v pid=%v",
			key.scope, key.ownerID, ownerPlayerID(key))
	}
}

// finishDeferredSaves 等待归属数据加载完成后执行排队的全量存盘。
func (actor *Activity) finishDeferredSaves(ctx tree.Context) {
	if actor == nil || actor.ownerLoads > 0 || actor.lifecycle == nil {
		return
	}
	requests := actor.pendingSaves
	actor.pendingSaves = nil
	if actor.shutdownSave != nil {
		requests = append(requests, actor.shutdownSave)
		actor.shutdownSave = nil
	}
	if len(requests) == 0 {
		return
	}
	actor.lifecycle.SaveAll(ctx, func(_ tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[activity/Activity.finishDeferredSaves] 延迟活动存盘失败, err=%v", err)
		}
		for _, request := range requests {
			if request != nil {
				request.Respond(nil, err)
			}
		}
		if err == nil {
			for key, state := range actor.ownerStates {
				if state != nil && state.releaseRequested && !state.online {
					actor.releaseOwnerState(key, state)
				}
			}
		}
	})
}

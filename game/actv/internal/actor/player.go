package actor

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// maxPendingPlayerRequests 表示单个玩家活动数据加载期间暂存事件和请求的总数量上限。
const maxPendingPlayerRequests = 64

// playerLoadState 记录单个玩家活动数据加载/释放过程中的状态和暂存消息。
// “是否已加载”不在此记录，统一查询 service.IsPlayerLoaded。
// 状态对象被替换或删除后，旧加载回调通过指针比对识别为过期并丢弃。
type playerLoadState struct {
	loading   bool                   // 正在加载玩家活动数据。
	releasing bool                   // 正在存盘释放的数据，完成前不发起新加载。
	reload    bool                   // 释放完成后需要重新加载。
	pending   []pendingActvRequest   // 玩家数据就绪前按顺序暂存的事件与请求。
	metas     *[]service.ActvMeta    // 已返回的玩家独有活动元数据。
	docs      *[]service.ActvDataDoc // 已返回的随玩家加载的组件数据。
}

// ensurePlayerLoaded 返回玩家加载状态，并在未加载时发起加载。
func (actor *Actv) ensurePlayerLoaded(ctx tree.Context, uid uint64) *playerLoadState {
	state := actor.players[uid]
	if state == nil {
		state = &playerLoadState{}
		actor.players[uid] = state
	}
	if actor.actvService.IsPlayerLoaded(uid) || state.loading {
		return state
	}
	if state.releasing {
		state.reload = true
		return state
	}
	actor.startPlayerLoad(ctx, uid, state)
	return state
}

// startPlayerLoad 先加载玩家全部活动元数据以检查创建历史，再限定组件数据的加载范围。
func (actor *Actv) startPlayerLoad(ctx tree.Context, uid uint64, state *playerLoadState) {
	state.loading = true
	metaFilter := bson.M{
		"owner_scope": int32(service.ScopePlayer), "owner_id": int64(uid),
	}
	if !actor.lifecycle.LoadAll(
		ctx, (*service.ActvMeta)(nil), metaFilter,
		func(callbackCtx tree.Context, value interface{}, err error) {
			if actor.players[uid] != state || !state.loading {
				return
			}
			if err != nil {
				actor.onPlayerLoadFinished(
					callbackCtx, uid, state, err,
				)
				return
			}
			metas, ok := value.(*[]service.ActvMeta)
			if !ok || metas == nil {
				actor.onPlayerLoadFinished(
					callbackCtx, uid, state, fmt.Errorf("invalid player actv load result %T", value),
				)
				return
			}
			state.metas = metas
			actor.loadPlayerComponents(callbackCtx, uid, state)
		},
	) {
		actor.onPlayerLoadFinished(
			ctx, uid, state, fmt.Errorf("player actv load request could not be sent"),
		)
	}
}

// loadPlayerComponents 只读取有效个人活动及相关共享活动中的玩家条目，避免加载过期历史数据。
func (actor *Actv) loadPlayerComponents(ctx tree.Context, uid uint64, state *playerLoadState) {
	now := time.Now().Unix()
	actvIDs := actor.actvService.SharedActvIDs(actor.playerGuildID(uid), now)
	for index := range *state.metas {
		meta := &(*state.metas)[index]
		if !meta.IsExpired(now) {
			actvIDs = append(actvIDs, meta.GetActvID())
		}
	}
	if len(actvIDs) == 0 {
		actor.onPlayerLoadFinished(
			ctx, uid, state, nil,
		)
		return
	}
	dataFilter := bson.M{
		"owner_scope": int32(service.ScopePlayer), "owner_id": int64(uid),
		"actv_id": bson.M{"$in": actvIDs},
	}
	if !actor.lifecycle.LoadAll(
		ctx, (*service.ActvDataDoc)(nil), dataFilter,
		func(callbackCtx tree.Context, value interface{}, err error) {
			if err == nil {
				docs, ok := value.(*[]service.ActvDataDoc)
				if !ok || docs == nil {
					err = fmt.Errorf("invalid player actv data load result %T", value)
				}
				state.docs = docs
			}
			actor.onPlayerLoadFinished(
				callbackCtx, uid, state, err,
			)
		},
	) {
		actor.onPlayerLoadFinished(
			ctx, uid, state, fmt.Errorf("player actv data load request could not be sent"),
		)
	}
}

// onPlayerLoadFinished 在加载结束后恢复数据、检测活动创建，再按顺序分发暂存事件与请求。
func (actor *Actv) onPlayerLoadFinished(
	ctx tree.Context, uid uint64, state *playerLoadState, err error,
) {
	if actor.players[uid] != state || !state.loading {
		return // 加载期间玩家已释放，结果作废。
	}
	state.loading = false
	if err != nil {
		tlog.Log.Error("[actv/Actv.onPlayerLoadFinished] 玩家活动数据加载失败, pid=%v err=%v", uid, err)
		delete(actor.players, uid)
		respondPending(ctx, state.pending, err)
		return
	}
	actor.applyPlayerData(uid, state)
	actor.initializePlayerActvs(uid, state)
	state.metas, state.docs = nil, nil
	actor.settleEndedPlayer(ctx, uid, state)
	pending := state.pending
	state.pending = nil
	for _, item := range pending {
		actor.replayPending(ctx, item)
	}

}

// settleEndedPlayer 对已结束活动调用模块结算，结算结果存盘成功后卸载该玩家条目。
// 存盘期间玩家被释放时，条目由释放流程存盘卸载，回调识别为过期后不再处理。
func (actor *Actv) settleEndedPlayer(ctx tree.Context, uid uint64, state *playerLoadState) {
	docs, settled := actor.actvService.SettleEndedPlayer(uid, actor.playerGuildID(uid))
	if len(docs) == 0 {
		return
	}
	actor.saveDocs(ctx, "settle", uid, nil, docs, func(callbackCtx tree.Context) {
		if actor.players[uid] != state || !actor.actvService.IsPlayerLoaded(uid) {
			return
		}
		actor.actvService.UnloadEndedPlayer(uid, settled)
	})
}

// applyPlayerData 恢复玩家独有活动实例，并将组件数据挂入对应活动的组件。
func (actor *Actv) applyPlayerData(uid uint64, state *playerLoadState) {
	restored := make([]*service.Actv, 0)
	if state.metas != nil {
		for index := range *state.metas {
			meta := &(*state.metas)[index]
			if meta.GetOwnerScope() != service.ScopePlayer || uint64(meta.GetOwnerID()) != uid {
				continue
			}
			if actor.actvService.GetActvByID(meta.GetActvID()) != nil {
				continue
			}
			if actor.restoreActv(meta) {
				restored = append(restored, actor.actvService.GetActvByID(meta.GetActvID()))
			}
		}
	}
	actor.actvService.SetPlayerLoaded(uid)
	if state.docs != nil {
		actor.actvService.ApplyDataDocs(*state.docs, actor.playerGuildID(uid))
	}
	for _, actv := range restored {
		actor.actvService.Loaded(actv)
	}
	actor.actvService.NotifyPlayerLoaded(uid, actor.playerGuildID(uid))
	tlog.Log.Info("[actv/Actv.applyPlayerData] 玩家活动数据加载完成, pid=%v actvs=%v data=%v",
		uid, len(restored), docCount(state.docs))
}

func docCount(docs *[]service.ActvDataDoc) int {
	if docs == nil {
		return 0
	}
	return len(*docs)
}

// initializePlayer 是唯一允许启动玩家活动加载的入口，业务事件和请求不调用此方法。
func (actor *Actv) initializePlayer(ctx tree.Context, event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	if uid == 0 {
		logPlayerInputFailure(event, fmt.Errorf("player initialization requires uid"))
		return
	}
	actor.ensurePlayerLoaded(ctx, uid)
}

// dispatchPlayerRequest 只处理请求：初始化期间暂存，未初始化则拒绝，不隐式加载数据。
func (actor *Actv) dispatchPlayerRequest(ctx tree.Context, message *ipb.PlayerActvMessage) {
	uid := message.GetUID()
	if actor.actvService.IsPlayerLoaded(uid) {
		actor.handlePlayerActvMessage(ctx, message)
		return
	}
	if state := actor.players[uid]; state != nil && (state.loading || state.reload) {
		state.queuePending(message)
		return
	}
	logActvFailure(message, fmt.Errorf("player actv is not initialized"))
}

// queuePending 初始化只暂存原始消息，沿用收件顺序，事件与请求各自由独立入口回放。
func (state *playerLoadState) queuePending(message interface{}) {
	if len(state.pending) >= maxPendingPlayerRequests {
		logPlayerInputFailure(message, fmt.Errorf("player actv initialization queue full"))
		return
	}
	state.pending = append(state.pending, pendingActvRequest{message: message})
}

// replayPending 只恢复初始化期间的收件顺序，不解析 UID、不加载数据、不执行模板业务。
func (actor *Actv) replayPending(ctx tree.Context, item pendingActvRequest) {
	switch message := item.message.(type) {
	case *sspb.Event:
		actor.onEvent(ctx, message)
	case *ipb.PlayerActvMessage:
		actor.onPlayerActvMessage(ctx, message)
	}
}

// logPlayerInputFailure 记录事件或请求加载失败，保留事件归属及名称便于定位。
func logPlayerInputFailure(message interface{}, err error) {
	if event, ok := message.(*sspb.Event); ok {
		tlog.Log.Error(
			"[actv/logPlayerInputFailure] 玩家活动事件处理失败, pid=%v event=%v err=%v",
			gameevent.PlayerUID(event), event.GetName(), err,
		)
		return
	}
	request, _ := message.(*ipb.PlayerActvMessage)
	logActvFailure(request, err)
}

// onPlayerOwnerReleased 存盘玩家相关的脏数据并从内存卸载。
// 存盘完成前该玩家的新加载会被推迟，避免读到旧数据。
func (actor *Actv) onPlayerOwnerReleased(ctx tree.Context, message *sspb.Event) {
	uid := gameevent.PlayerUID(message)
	if uid == 0 {
		return
	}
	state := actor.players[uid]
	if state == nil || state.loading || state.releasing {
		return
	}
	actor.actvService.NotifyPlayerUnload(uid, actor.playerGuildID(uid))
	metas := actor.actvService.PlayerDirtyMetas(uid)
	docs := actor.actvService.PlayerDirtyData(uid, actor.playerGuildID(uid))
	actor.actvService.UnloadPlayer(uid, actor.playerGuildID(uid))
	released := &playerLoadState{releasing: true}
	actor.players[uid] = released
	actor.saveDocs(ctx, "release", uid, metas, docs, func(callbackCtx tree.Context) {
		if actor.players[uid] != released {
			return
		}
		released.releasing = false
		if !released.reload {
			delete(actor.players, uid)
			return
		}
		released.reload = false
		actor.startPlayerLoad(callbackCtx, uid, released)
	})
}

// saveDocs 存盘一批元数据和组件数据快照，全部完成（无论成败）后回调；reason 仅用于日志。
// 失败的数据仍为脏数据，由定时存盘继续处理。
func (actor *Actv) saveDocs(
	ctx tree.Context,
	reason string,
	uid uint64,
	metas []*service.ActvMeta,
	docs []*service.ActvDataDoc,
	done func(tree.Context),
) {
	remaining := len(metas) + len(docs)
	if remaining == 0 {
		done(ctx)
		return
	}
	finish := func(callbackCtx tree.Context, err error) {
		if err != nil {
			tlog.Log.Error("[actv/Actv.saveDocs] 玩家活动数据存盘失败, reason=%v pid=%v err=%v", reason, uid, err)
		}
		remaining--
		if remaining == 0 {
			tlog.Log.Info("[actv/Actv.saveDocs] 玩家活动数据存盘完成, reason=%v pid=%v metas=%v data=%v",
				reason, uid, len(metas), len(docs))
			done(callbackCtx)
		}
	}
	for _, meta := range metas {
		if !actor.lifecycle.Save(ctx, (*service.ActvMeta)(nil), meta, finish) {
			finish(ctx, fmt.Errorf("actv meta save not scheduled, actv=%v", meta.GetActvID()))
		}
	}
	for _, doc := range docs {
		if !actor.lifecycle.Save(ctx, (*service.ActvDataDoc)(nil), doc, finish) {
			finish(ctx, fmt.Errorf("actv data save not scheduled, id=%v", doc.GetID()))
		}
	}
}

// respondPending 统一结束加载失败或已释放玩家的暂存请求。
func respondPending(ctx tree.Context, pending []pendingActvRequest, err error) {
	for _, item := range pending {
		logPlayerInputFailure(item.message, err)
	}
}

// playerGuildID 返回玩家所属联盟编号，用于按归属索引定位与玩家相关的联盟活动。
// 活动 Actor 目前还拿不到联盟成员关系，统一返回 0：只处理服务器活动，联盟活动的玩家条目不加载、不通知。
// 接入成员关系后在此返回真实联盟编号；加载与释放必须使用同一编号，保证条目能完整卸载。
func (actor *Actv) playerGuildID(uint64) service.OwnerID {
	return 0
}

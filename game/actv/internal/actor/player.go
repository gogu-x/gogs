package actor

import (
	"fmt"

	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// maxPendingPlayerRequests 表示单个玩家活动数据加载期间暂存请求的数量上限。
const maxPendingPlayerRequests = 64

// playerLoadState 记录单个玩家活动数据加载/释放过程中的状态和暂存请求。
// “是否已加载”不在此记录，统一查询 service.IsPlayerLoaded。
// 状态对象被替换或删除后，旧加载回调通过指针比对识别为过期并丢弃。
type playerLoadState struct {
	loading   bool                     // 正在加载玩家活动数据。
	releasing bool                     // 正在存盘释放的数据，完成前不发起新加载。
	reload    bool                     // 释放完成后需要重新加载。
	pending   []pendingActivityRequest // 玩家数据就绪前暂存的请求。
	metas     *[]service.ActvMeta      // 已返回的玩家独有活动元数据。
	docs      *[]service.ActvDataDoc   // 已返回的随玩家加载的组件数据。
	remaining int                      // 尚未返回的加载请求数。
	loadErr   error                    // 首个加载错误。
}

// onPlayerSessionOpened 玩家上线时加载其活动数据。
func (actor *Activity) onPlayerSessionOpened(ctx tree.Context, message *ipb.PlayerSessionOpened) {
	uid := message.GetUID()
	if uid == 0 {
		return
	}
	if !actor.ready {
		actor.pendingLogins[uid] = true
		return
	}
	actor.ensurePlayerLoaded(ctx, uid)
}

// ensurePlayerLoaded 返回玩家加载状态，并在未加载时发起加载。
func (actor *Activity) ensurePlayerLoaded(ctx tree.Context, uid uint64) *playerLoadState {
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

// startPlayerLoad 并行加载玩家独有活动元数据和随玩家加载的全部组件数据。
func (actor *Activity) startPlayerLoad(ctx tree.Context, uid uint64, state *playerLoadState) {
	state.loading = true
	state.remaining = 2
	state.loadErr = nil
	metaFilter := bson.M{"owner_scope": int32(service.ScopePlayer), "owner_id": int64(uid)}
	if !actor.lifecycle.LoadAll(ctx, (*service.ActvMeta)(nil), metaFilter, func(callbackCtx tree.Context, value interface{}, err error) {
		if err == nil {
			metas, ok := value.(*[]service.ActvMeta)
			if !ok {
				err = fmt.Errorf("invalid player activity load result %T", value)
			}
			state.metas = metas
		}
		actor.onPlayerPartLoaded(callbackCtx, uid, state, err)
	}) {
		actor.onPlayerPartLoaded(ctx, uid, state, fmt.Errorf("player activity load request could not be sent"))
	}
	dataFilter := bson.M{"owner_scope": int32(service.ScopePlayer), "owner_id": int64(uid)}
	if !actor.lifecycle.LoadAll(ctx, (*service.ActvDataDoc)(nil), dataFilter, func(callbackCtx tree.Context, value interface{}, err error) {
		if err == nil {
			docs, ok := value.(*[]service.ActvDataDoc)
			if !ok {
				err = fmt.Errorf("invalid player activity data load result %T", value)
			}
			state.docs = docs
		}
		actor.onPlayerPartLoaded(callbackCtx, uid, state, err)
	}) {
		actor.onPlayerPartLoaded(ctx, uid, state, fmt.Errorf("player activity data load request could not be sent"))
	}
}

// onPlayerPartLoaded 汇总两段加载结果，全部返回后恢复玩家数据并放行暂存请求。
func (actor *Activity) onPlayerPartLoaded(ctx tree.Context, uid uint64, state *playerLoadState, err error) {
	if err != nil && state.loadErr == nil {
		state.loadErr = err
	}
	state.remaining--
	if state.remaining > 0 {
		return
	}
	if actor.players[uid] != state || !state.loading {
		return // 加载期间玩家已释放，结果作废。
	}
	state.loading = false
	if state.loadErr != nil {
		tlog.Log.Error("[activity/Activity.onPlayerPartLoaded] 玩家活动数据加载失败, pid=%v err=%v", uid, state.loadErr)
		delete(actor.players, uid)
		respondPending(state.pending, state.loadErr)
		return
	}
	actor.applyPlayerData(uid, state)
	state.metas, state.docs = nil, nil
	actor.settleEndedPlayer(ctx, uid, state)
	pending := state.pending
	state.pending = nil
	for _, item := range pending {
		message, _ := item.message.(*ipb.PlayerActivityMessage)
		response, handleErr := actor.handlePlayerActivityMessage(message)
		item.request.Respond(response, handleErr)
	}
}

// settleEndedPlayer 对已结束活动调用模块结算，结算结果存盘成功后卸载该玩家条目。
// 存盘期间玩家被释放时，条目由释放流程存盘卸载，回调识别为过期后不再处理。
func (actor *Activity) settleEndedPlayer(ctx tree.Context, uid uint64, state *playerLoadState) {
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
func (actor *Activity) applyPlayerData(uid uint64, state *playerLoadState) {
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
			if actor.restoreActivity(meta) {
				restored = append(restored, actor.actvService.GetActvByID(meta.GetActvID()))
			}
		}
	}
	actor.actvService.SetPlayerLoaded(uid)
	if state.docs != nil {
		actor.actvService.ApplyDataDocs(*state.docs, actor.playerGuildID(uid))
	}
	for _, actv := range restored {
		actv.GetExt().OnLoaded()
	}
	actor.actvService.NotifyPlayerLoaded(uid, actor.playerGuildID(uid))
	tlog.Log.Info("[activity/Activity.applyPlayerData] 玩家活动数据加载完成, pid=%v activities=%v data=%v",
		uid, len(restored), docCount(state.docs))
}

func docCount(docs *[]service.ActvDataDoc) int {
	if docs == nil {
		return 0
	}
	return len(*docs)
}

// dispatchPlayerRequest 玩家数据已就绪时直接处理请求，否则发起加载并按玩家暂存。
func (actor *Activity) dispatchPlayerRequest(ctx tree.Context, request *tree.Envelope, message *ipb.PlayerActivityMessage) {
	uid := message.GetUID()
	if uid == 0 {
		response, err := actor.handlePlayerActivityMessage(message)
		request.Respond(response, err)
		return
	}
	state := actor.ensurePlayerLoaded(ctx, uid)
	if actor.players[uid] != state {
		request.Respond(nil, fmt.Errorf("player activity data load failed"))
		return
	}
	if actor.actvService.IsPlayerLoaded(uid) {
		response, err := actor.handlePlayerActivityMessage(message)
		request.Respond(response, err)
		return
	}
	if len(state.pending) >= maxPendingPlayerRequests {
		request.Respond(nil, fmt.Errorf("player activity data is still loading"))
		return
	}
	state.pending = append(state.pending, pendingActivityRequest{message: message, request: request})
}

// onPlayerOwnerReleased 存盘玩家相关的脏数据并从内存卸载。
// 存盘完成前该玩家的新加载会被推迟，避免读到旧数据。
func (actor *Activity) onPlayerOwnerReleased(ctx tree.Context, message *ipb.PlayerActivityOwnerReleased) {
	uid := message.GetUID()
	if uid == 0 {
		return
	}
	if !actor.ready {
		delete(actor.pendingLogins, uid)
		actor.dropGlobalPending(uid)
		return
	}
	state := actor.players[uid]
	if state == nil || state.releasing {
		return
	}
	if state.loading {
		delete(actor.players, uid)
		respondPending(state.pending, fmt.Errorf("player activity owner released"))
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
func (actor *Activity) saveDocs(
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
			tlog.Log.Error("[activity/Activity.saveDocs] 玩家活动数据存盘失败, reason=%v pid=%v err=%v", reason, uid, err)
		}
		remaining--
		if remaining == 0 {
			tlog.Log.Info("[activity/Activity.saveDocs] 玩家活动数据存盘完成, reason=%v pid=%v metas=%v data=%v",
				reason, uid, len(metas), len(docs))
			done(callbackCtx)
		}
	}
	for _, meta := range metas {
		if !actor.lifecycle.Save(ctx, (*service.ActvMeta)(nil), meta, finish) {
			finish(ctx, fmt.Errorf("activity meta save not scheduled, activity=%v", meta.GetActvID()))
		}
	}
	for _, doc := range docs {
		if !actor.lifecycle.Save(ctx, (*service.ActvDataDoc)(nil), doc, finish) {
			finish(ctx, fmt.Errorf("activity data save not scheduled, id=%v", doc.GetID()))
		}
	}
}

// dropGlobalPending 拒绝启动加载期间该玩家暂存的请求，避免释放后再被加载而无人释放。
func (actor *Activity) dropGlobalPending(uid uint64) {
	kept := actor.pending[:0]
	for _, item := range actor.pending {
		message, ok := item.message.(*ipb.PlayerActivityMessage)
		if ok && message.GetUID() == uid {
			item.request.Respond(nil, fmt.Errorf("player activity owner released"))
			continue
		}
		kept = append(kept, item)
	}
	actor.pending = kept
}

func respondPending(pending []pendingActivityRequest, err error) {
	for _, item := range pending {
		if item.request != nil {
			item.request.Respond(nil, err)
		}
	}
}

// playerGuildID 返回玩家所属联盟编号，用于按归属索引定位与玩家相关的联盟活动。
// 活动 Actor 目前还拿不到联盟成员关系，统一返回 0：只处理服务器活动，联盟活动的玩家条目不加载、不通知。
// 接入成员关系后在此返回真实联盟编号；加载与释放必须使用同一编号，保证条目能完整卸载。
func (actor *Activity) playerGuildID(uint64) service.OwnerID {
	return 0
}

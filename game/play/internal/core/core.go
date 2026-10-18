// core 包提供单玩家角色的运行期状态与单次请求上下文。
package core

import (
	"errors"
	"fmt"

	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	// playerMailboxSize 表示玩家角色消息队列的容量。
	playerMailboxSize = 512
	// maxPendingMessages 表示玩家数据加载期间可暂存消息的上限。
	maxPendingMessages = 512
)

// PlayerActor 独占一名玩家的数据和业务消息队列。
//
// 玩家状态只在所属协程内访问；网关负责按玩家编号建立和复用该角色。
type PlayerActor struct {
	UID    uint64             // 当前 Actor 所属玩家 ID。
	Player *player.Player     // 当前玩家的内存数据。
	Event  *comm.Event        // 当前玩家的业务事件分发器。
	Boot   func(*PlayerActor) // Boot 在路由与事件初始化阶段调用。

	initialMessages    []interface{}
	router             tree.Router
	messageRouter      tree.Router
	wheel              *timer.TimeWheel
	idleTimer          *timer.WheelTimer
	system             *tree.Tree
	selfPID            tree.PID
	store              *mongorpc.Store
	model              *player.PersistenceModel
	gatePID            tree.PID
	ready              bool
	loadErr            error
	pending            []interface{}
	online             bool
	sessionActive      bool
	sessionGeneration  uint64
	saving             bool
	evictAfterSave     bool
	stopRequested      bool
	shutdownStop       bool
	shutdownRequest    *tree.Envelope
	idleGeneration     uint64
	evictReadySent     bool
	accepting          bool
	serverOpenEmitted  bool
	serverCloseEmitted bool
	saveCallbacks      []func(error)
}

// Name 返回当前玩家角色的运行期名称。
func (actor *PlayerActor) Name() string { return fmt.Sprintf("player-%d", actor.UID) }

// MailboxSize 为单名玩家提供有界消息缓冲。
func (actor *PlayerActor) MailboxSize() int { return playerMailboxSize }

// OnInit 初始化玩家业务路由，并异步加载玩家数据。
func (actor *PlayerActor) OnInit(ctx tree.Context) {
	actor.registerMessageHandlers()
	actor.Event = comm.NewEvent()
	actor.accepting = true
	actor.gatePID, _ = ctx.Lookup(def.GameGate)
	actor.system = ctx.System()
	actor.selfPID = ctx.Self()
	actor.registerTimers()
	actor.scheduleIdleEviction()
	uid := actor.UID
	for _, message := range actor.initialMessages {
		if !actor.addPending(message) {
			tlog.Log.Error("[play/PlayerActor.OnInit] 初始玩家消息超出队列上限, pid=%v", uid)
		}
	}
	if actor.Boot != nil {
		actor.Boot(actor)
	}
	if uid == 0 {
		actor.failLoad(ctx, errors.New("player actor requires a non-zero uid"))
		return
	}
	if actor.Player != nil {
		actor.model = player.NewPersistenceModel(actor.Player)
		actor.ready = true
		actor.scheduleAutoSave()
		return
	}

	actor.model = player.NewPersistenceModel(nil)
	actor.store = mongorpc.NewStore(def.Mongo)
	if err := actor.store.Register(actor.model); err != nil {
		actor.failLoad(ctx, err)
		return
	}
	if !actor.store.LoadOne(
		ctx,
		(*player.Player)(nil),
		uid,
		actor.onPlayerLoaded,
	) {
		actor.failLoad(ctx, errors.New("player load request could not be sent"))
	}
}

// OnStop 通知网关回收玩家编号到角色进程编号的映射。
func (actor *PlayerActor) OnStop(ctx tree.Context) {
	if actor.wheel != nil {
		actor.wheel.Stop()
	}
	if actor.gatePID.ID != 0 {
		ctx.Send(actor.gatePID, &ipb.PlayerActorStopped{UID: actor.UID, PID: ctx.Self()})
	}
}

// Router 返回玩家业务路由，供装配层注册请求处理器；角色系统消息由内部路由处理。
func (actor *PlayerActor) Router() *tree.Router { return &actor.router }

// SetInitialMessages 保存玩家数据加载完成前需要排队处理的系统消息。
func (actor *PlayerActor) SetInitialMessages(messages []interface{}) {
	if actor == nil {
		return
	}
	actor.initialMessages = append([]interface{}(nil), messages...)
}

// GetPlayer 返回当前角色独占的玩家数据。
func (actor *PlayerActor) GetPlayer() *player.Player {
	if actor == nil {
		return nil
	}
	return actor.Player
}

// OnEvent 注册当前玩家角色的业务事件监听器。
func (actor *PlayerActor) OnEvent(name comm.EventName, fn EventFunc, priority ...int) {
	if fn == nil {
		return
	}
	actor.Event.Register(name, func(arg *comm.Arg) {
		fn(actor, arg)
	}, priority...)
}

// Emit 在当前玩家角色所属协程中同步触发业务事件。
func (actor *PlayerActor) Emit(name comm.EventName, arg *comm.Arg) int {
	if actor == nil || actor.Event == nil {
		return 0
	}
	return actor.Event.Emit(name, arg)
}

// MarkOnline 将玩家标记为已建立客户端会话。
func (actor *PlayerActor) MarkOnline() {
	actor.online = true
	actor.evictAfterSave = false
	actor.stopRequested = false
	actor.evictReadySent = false
	actor.cancelIdleEviction()
}

// onPlayerLoaded 校验已加载的数据，为新玩家初始化数据，并处理排队中的消息。
func (actor *PlayerActor) onPlayerLoaded(ctx tree.Context, value interface{}, err error) {
	uid := actor.UID
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
		value = nil
	}
	if err != nil {
		actor.failLoad(ctx, err)
		return
	}

	var loaded *player.Player
	if value != nil {
		var valid bool
		loaded, valid = value.(*player.Player)
		if !valid || loaded == nil || loaded.GetUID() != uid {
			actor.failLoad(ctx, fmt.Errorf("invalid player document for uid=%d", uid))
			return
		}
	}
	if loaded == nil {
		loaded = player.NewPlayerData(uid)
		tlog.Log.Info("[play/PlayerActor.onPlayerLoaded] 新玩家数据已初始化, pid=%v", uid)
	}
	actor.Player = loaded
	actor.model.SetPlayer(loaded)
	actor.ready = true
	actor.scheduleAutoSave()
	actor.drainPending(ctx)
	if len(actor.saveCallbacks) > 0 {
		actor.save(ctx)
	} else if actor.shutdownStop || actor.evictAfterSave && !actor.online {
		actor.save(ctx)
	}
}

// failLoad 将加载状态标记为失败，并通知等待中的登录、存盘和回收请求。
func (actor *PlayerActor) failLoad(ctx tree.Context, err error) {
	uid := actor.UID
	gatePID := actor.gatePID
	actor.loadErr = err
	tlog.Log.Error("[play/PlayerActor.failLoad] 玩家数据加载失败, pid=%v err=%v", uid, err)
	for _, message := range actor.takePending() {
		if actor.loginRequest(message) != nil {
			actor.sendLoginFailure(ctx, pb_common.ErrCode_INTERNAL)
		}
	}
	actor.ready = false
	actor.evictAfterSave = true
	if gatePID.ID != 0 {
		ctx.Send(gatePID, &ipb.PlayerLoadFailed{UID: uid, PID: ctx.Self(), Reason: err.Error()})
	}
	actor.resolveSaveCallbacks(err)
	if actor.shutdownStop {
		actor.finishShutdownStop(ctx, err)
		return
	}
	actor.notifyEvictReady(ctx)
}

// queueWhileLoading 在玩家数据就绪前暂存消息，或在加载失败后拒绝处理。
func (actor *PlayerActor) queueWhileLoading(ctx tree.Context, msg interface{}) {
	uid := actor.UID
	if actor.loadErr != nil {
		if actor.loginRequest(msg) != nil {
			actor.sendLoginFailure(ctx, pb_common.ErrCode_INTERNAL)
		}
		return
	}
	if len(actor.pending) >= maxPendingMessages {
		tlog.Log.Error("[play/PlayerActor.queueWhileLoading] 玩家加载期间请求队列已满, pid=%v limit=%v", uid, maxPendingMessages)
		if actor.loginRequest(msg) != nil {
			actor.sendLoginFailure(ctx, pb_common.ErrCode_INTERNAL)
		}
		return
	}
	actor.addPending(msg)
}

// addPending 在有界玩家消息队列未满时追加一条消息。
func (actor *PlayerActor) addPending(msg interface{}) bool {
	if len(actor.pending) >= maxPendingMessages {
		return false
	}
	actor.pending = append(actor.pending, msg)
	return true
}

// drainPending 在玩家数据加载完成后分发已暂存的消息。
func (actor *PlayerActor) drainPending(ctx tree.Context) {
	pending := actor.takePending()
	for _, msg := range pending {
		actor.messageRouter.Route(ctx, msg)
	}
}

// onServerEvent 处理投递给当前玩家角色的服务器生命周期事件。
func (actor *PlayerActor) onServerEvent(ctx tree.Context, message *sspb.Event) {
	if !actor.ready {
		actor.queueWhileLoading(ctx, message)
		return
	}
	if gameevent.IsServerOpened(message) {
		actor.onServerOpened()
		return
	}
	if gameevent.IsServerClosing(message) {
		actor.onServerClosing()
		return
	}
	eventName := message.GetName()
	err := fmt.Errorf("unsupported player event %q", eventName)
	tlog.Log.Error("[play/PlayerActor.onServerEvent] 收到不支持的玩家事件, event=%v pid=%v err=%v", eventName, actor.UID, err)
}

// onSavePlayer 保存当前玩家数据，并可在完成后发送回复。
func (actor *PlayerActor) onSavePlayer(ctx tree.Context, message *ipb.SavePlayer) {
	if message == nil {
		err := errors.New("save player message is nil")
		tlog.Log.Error("[play/PlayerActor.onSavePlayer] 收到无效存盘消息, pid=%v err=%v", actor.UID, err)
		ctx.Response(nil, err)
		return
	}
	messageUID := message.UID
	if messageUID != 0 && messageUID != actor.UID {
		return
	}
	if message.Wait {
		request := ctx.RequestEnvelope()
		actor.saveWithCallback(ctx, func(err error) {
			if request != nil {
				request.Respond(nil, err)
			}
		})
		return
	}
	actor.save(ctx)
}

// onStopAcceptingPlayer 拒绝发给当前玩家角色的新客户端请求。
func (actor *PlayerActor) onStopAcceptingPlayer(_ tree.Context, message *ipb.StopAcceptingPlayer) {
	if message == nil {
		tlog.Log.Error("[play/PlayerActor.onStopAcceptingPlayer] 收到无效停止接入消息, pid=%v", actor.UID)
		return
	}
	messageUID := message.UID
	if messageUID == 0 || messageUID == actor.UID {
		actor.accepting = false
	}
}

// onServerOpened 仅触发一次玩家的服务器开启事件。
func (actor *PlayerActor) onServerOpened() {
	if actor.serverOpenEmitted {
		return
	}
	actor.serverOpenEmitted = true
	arg := comm.NewArg()
	arg.Set("uid", actor.UID)
	actor.Emit(PlayerOnServerOpen, arg)
	tlog.Log.Info("[play/PlayerActor.onServerOpened] 玩家 Actor 已处理开服事件, pid=%v", actor.UID)
}

// onServerClosing 仅触发一次玩家的服务器关闭事件。
func (actor *PlayerActor) onServerClosing() {
	if actor.serverCloseEmitted {
		return
	}
	actor.serverCloseEmitted = true
	arg := comm.NewArg()
	arg.Set("uid", actor.UID)
	actor.Emit(PlayerOnServerClose, arg)
	tlog.Log.Info("[play/PlayerActor.onServerClosing] 玩家 Actor 已处理关服事件, pid=%v", actor.UID)
}

// loginRequest 从直接消息或封装消息中提取登录请求。
func (actor *PlayerActor) loginRequest(msg interface{}) *pb_auth.LoginGameReq {
	if inbound, ok := msg.(*ipb.PlayerInbound); ok && inbound != nil {
		msg = inbound.Message
	}
	login, _ := msg.(*pb_auth.LoginGameReq)
	return login
}

// sendLoginFailure 通过网关推送登录失败响应。
func (actor *PlayerActor) sendLoginFailure(ctx tree.Context, code pb_common.ErrCode) {
	uid := actor.UID
	actor.push(ctx, uid, &pb_auth.LoginGameAck{Code: code, Uid: uid})
}

// onSessionOpened 将新会话代次绑定到当前玩家角色。
func (actor *PlayerActor) onSessionOpened(_ tree.Context, message *ipb.PlayerSessionOpened) {
	uid := actor.UID
	if message == nil || message.UID != uid || message.Generation <= actor.sessionGeneration {
		return
	}
	actor.sessionGeneration = message.Generation
	actor.sessionActive = true
	actor.MarkOnline()
	tlog.Log.Info("[play/PlayerActor.onSessionOpened] 玩家会话已绑定, pid=%v generation=%v", uid, actor.sessionGeneration)
}

// onPlayerInbound 校验网关会话，并分发其中的客户端消息。
func (actor *PlayerActor) onPlayerInbound(ctx tree.Context, message *ipb.PlayerInbound) {
	uid := actor.UID
	if actor.serverCloseEmitted {
		return
	}
	if message == nil || message.UID != uid || message.Generation != actor.sessionGeneration || !actor.sessionActive {
		if message != nil {
			tlog.Log.Error("[play/PlayerActor.onPlayerInbound] 忽略过期或跨玩家请求, pid=%v generation=%v currentGeneration=%v",
				uid, message.Generation, actor.sessionGeneration)
		}
		return
	}
	if !actor.accepting {
		return
	}
	if !actor.ready {
		actor.queueWhileLoading(ctx, message)
		return
	}
	actor.routeMessage(ctx, message.Message)
}

// takePending 取出并返回当前加载队列。
func (actor *PlayerActor) takePending() []interface{} {
	pending := actor.pending
	actor.pending = nil
	return pending
}

// routeMessage 校验玩家归属，分发业务请求，并安排空闲回收。
func (actor *PlayerActor) routeMessage(ctx tree.Context, msg interface{}) {
	uid := actor.UID
	actor.evictReadySent = false
	if login, ok := msg.(*pb_auth.LoginGameReq); ok {
		loginUID := login.GetUID()
		if loginUID != uid {
			tlog.Log.Error("[play/PlayerActor.routeMessage] 登录 UID 与 Actor 不匹配, pid=%v requestUID=%v", uid, loginUID)
			actor.sendLoginFailure(ctx, pb_common.ErrCode_PARAM)
			return
		}
		actor.MarkOnline()
	}
	if request, ok := msg.(interface{ GetUID() uint64 }); ok {
		requestUID := request.GetUID()
		if requestUID != uid {
			tlog.Log.Error("[play/PlayerActor.routeMessage] 玩家请求 UID 与 Actor 不匹配, pid=%v requestUID=%v type=%T", uid, requestUID, msg)
			ctx.Response(nil, fmt.Errorf("player actor uid mismatch: actor=%d request=%d", uid, requestUID))
			return
		}
	}

	actor.router.Route(ctx, msg)
	if _, ok := msg.(*pb_auth.LoginGameReq); ok {
		return
	}
	if !actor.online {
		actor.scheduleIdleEviction()
	}
}

// onSessionClosed 将匹配的会话标记为离线，触发登出事件，并保存玩家数据。
func (actor *PlayerActor) onSessionClosed(ctx tree.Context, message *ipb.SessionClosed) {
	uid := actor.UID
	if message == nil || message.UID != uid || message.Generation != actor.sessionGeneration || !actor.sessionActive {
		return
	}
	actor.sessionActive = false
	actor.online = false
	actor.sessionGeneration = message.Generation
	actor.evictAfterSave = false
	actor.stopRequested = false
	arg := comm.NewArg()
	arg.Set("uid", uid)
	actor.Emit(PlayerOnLogout, arg)
	tlog.Log.Info("[play/PlayerActor.onSessionClosed] 玩家会话已断开, pid=%v", uid)
	actor.scheduleIdleEviction()
	actor.save(ctx)
}

// save 保存当前玩家数据快照，且不设置完成回调。
func (actor *PlayerActor) save(ctx tree.Context) {
	actor.saveWithCallback(ctx, nil)
}

// saveWithCallback 保存已变更的玩家数据，并在存盘完成后执行回调。
func (actor *PlayerActor) saveWithCallback(ctx tree.Context, callback func(error)) {
	if callback != nil {
		actor.addSaveCallback(callback)
	}
	if !actor.ready {
		return
	}
	currentPlayer := actor.Player
	model := actor.model
	store := actor.store
	if currentPlayer == nil || model == nil || store == nil {
		actor.onSaveFailed(ctx, errors.New("player persistence store is unavailable"))
		return
	}
	if !currentPlayer.IsDirty() {
		actor.resolveSaveCallbacks(nil)
		actor.finishAfterSave(ctx)
		return
	}
	if actor.saving {
		return
	}
	actor.saving = true
	cloned := model.Clone(currentPlayer)
	snapshot, valid := cloned.(*player.Player)
	if !valid || snapshot == nil {
		tlog.Log.Error("[play/PlayerActor.save] 创建玩家存盘快照失败, pid=%v", actor.UID)
		actor.onSaveFailed(ctx, errors.New("could not create player save snapshot"))
		return
	}
	uid := actor.UID
	if !store.Save(ctx, (*player.Player)(nil), uid, snapshot, func(callbackCtx tree.Context, err error) {
		actor.onSaveComplete(callbackCtx, snapshot, err)
	}) {
		tlog.Log.Error("[play/PlayerActor.save] 玩家存盘请求未发送, pid=%v", uid)
		actor.onSaveFailed(ctx, errors.New("player save request could not be sent"))
	}
}

// onSaveComplete 更新存盘状态，并处理存盘期间产生的数据变更。
func (actor *PlayerActor) onSaveComplete(ctx tree.Context, snapshot *player.Player, err error) {
	uid := actor.UID
	model := actor.model
	currentPlayer := actor.Player
	actor.saving = false
	if err != nil {
		tlog.Log.Error("[play/PlayerActor.onSaveComplete] 玩家存盘失败, pid=%v err=%v", uid, err)
		actor.onSaveFailed(ctx, err)
		return
	}
	if err == nil {
		err = model.AfterSave(ctx, snapshot)
	}
	if err != nil {
		tlog.Log.Error("[play/PlayerActor.onSaveComplete] 存盘后更新脏标记失败, pid=%v err=%v", uid, err)
		actor.onSaveFailed(ctx, err)
		return
	}
	tlog.Log.Info("[play/PlayerActor.onSaveComplete] 玩家数据已存盘, pid=%v", uid)
	if currentPlayer.IsDirty() {
		actor.save(ctx)
		return
	}
	actor.resolveSaveCallbacks(nil)
	actor.finishAfterSave(ctx)
}

// addSaveCallback 将完成回调加入当前或下一次存盘任务。
func (actor *PlayerActor) addSaveCallback(callback func(error)) {
	if callback != nil {
		actor.saveCallbacks = append(actor.saveCallbacks, callback)
	}
}

// takeSaveCallbacks 取出并返回所有等待当前存盘任务的回调。
func (actor *PlayerActor) takeSaveCallbacks() []func(error) {
	callbacks := actor.saveCallbacks
	actor.saveCallbacks = nil
	return callbacks
}

// resolveSaveCallbacks 使用相同结果完成所有等待中的存盘回调。
func (actor *PlayerActor) resolveSaveCallbacks(err error) {
	for _, callback := range actor.takeSaveCallbacks() {
		callback(err)
	}
}

// stopIfSaved 在所需的登出处理和最终存盘完成后停止当前角色。
func (actor *PlayerActor) stopIfSaved(ctx tree.Context, message *ipb.StopPlayerActor) {
	if message == nil {
		return
	}
	messageUID := message.GetUID()
	if messageUID != 0 && messageUID != actor.UID {
		return
	}
	if message.GetShutdown() {
		actor.accepting = false
		actor.shutdownStop = true
		actor.shutdownRequest = ctx.RequestEnvelope()
		actor.evictAfterSave = false
		actor.cancelIdleEviction()
		if actor.sessionActive {
			actor.sessionActive = false
			actor.online = false
			arg := comm.NewArg()
			arg.Set("uid", actor.UID)
			actor.Emit(PlayerOnLogout, arg)
		}
		if actor.ready {
			actor.save(ctx)
		} else if actor.loadErr != nil {
			actor.finishShutdownStop(ctx, actor.loadErr)
		}
		return
	}
	if actor.online || actor.sessionActive {
		return
	}
	actor.stopRequested = true
	if actor.saving {
		return
	}
	if actor.Player != nil && actor.Player.IsDirty() {
		actor.save(ctx)
		return
	}
	ctx.Stop()
}

// sendTimerMessage 将定时器结果发送回当前角色的消息队列。
func (actor *PlayerActor) sendTimerMessage(message interface{}) {
	if actor.system == nil || actor.selfPID.ID == 0 || !actor.system.TrySend(actor.selfPID, message) {
		tlog.Log.Error("[play/PlayerActor.sendTimerMessage] 投递玩家定时消息失败, pid=%v type=%T", actor.UID, message)
	}
}

// scheduleAutoSave 在角色就绪后启动玩家定时存盘任务。
func (actor *PlayerActor) scheduleAutoSave() {
	if actor.wheel == nil || actor.store == nil || !actor.ready || actor.loadErr != nil {
		return
	}
	if actor.wheel.After(playerAutoSaveTimerType, playerAutoSaveInterval, nil) == nil {
		tlog.Log.Error("[play/PlayerActor.scheduleAutoSave] 安排玩家周期存盘失败, pid=%v", actor.UID)
	}
}

// onAutoSaveTick 保存已就绪的玩家数据，并安排下一次定时存盘。
func (actor *PlayerActor) onAutoSaveTick(ctx tree.Context, _ *playerAutoSaveTick) {
	if actor.ready && actor.loadErr == nil {
		actor.save(ctx)
	}
	actor.scheduleAutoSave()
}

// scheduleIdleEviction 为离线的玩家角色启动保留计时器。
func (actor *PlayerActor) scheduleIdleEviction() {
	if actor.wheel == nil || actor.online || actor.sessionActive {
		return
	}
	if actor.idleTimer != nil {
		actor.idleTimer.Stop()
	}
	actor.idleGeneration++
	generation := actor.idleGeneration
	actor.idleTimer = actor.wheel.After(playerIdleTimerType, playerOfflineRetention, generation)
	if actor.idleTimer == nil {
		tlog.Log.Error("[play/PlayerActor.scheduleIdleEviction] 安排玩家离线回收失败, pid=%v", actor.UID)
	}
}

// cancelIdleEviction 使当前离线保留计时器失效并停止计时。
func (actor *PlayerActor) cancelIdleEviction() {
	if actor.idleTimer != nil {
		actor.idleTimer.Stop()
		actor.idleTimer = nil
	}
	actor.idleGeneration++
}

// onIdleTimeout 在玩家离线保留期结束后启动最终存盘。
func (actor *PlayerActor) onIdleTimeout(ctx tree.Context, message *playerIdleTimeout) {
	if message == nil || message.generation != actor.idleGeneration {
		return
	}
	actor.idleTimer = nil
	if actor.online || actor.sessionActive {
		return
	}
	actor.evictAfterSave = true
	actor.stopRequested = false
	tlog.Log.Info("[play/PlayerActor.onIdleTimeout] 玩家离线保留期已到，开始最终存盘并申请回收, pid=%v retention=%v",
		actor.UID, playerOfflineRetention)
	actor.save(ctx)
}

// finishAfterSave 在存盘完成后停止当前角色或通知网关。
func (actor *PlayerActor) finishAfterSave(ctx tree.Context) {
	if actor.shutdownStop {
		actor.finishShutdownStop(ctx, nil)
		return
	}
	if actor.stopRequested && !actor.online && !actor.sessionActive {
		ctx.Stop()
		return
	}
	if actor.evictAfterSave && !actor.online && !actor.sessionActive {
		actor.notifyEvictReady(ctx)
	}
}

// onSaveFailed 完成等待存盘的请求，并更新关服或回收状态。
func (actor *PlayerActor) onSaveFailed(ctx tree.Context, err error) {
	actor.saving = false
	actor.resolveSaveCallbacks(err)
	if actor.shutdownStop {
		actor.finishShutdownStop(ctx, err)
		return
	}
	if actor.stopRequested {
		actor.stopRequested = false
		actor.evictAfterSave = true
		actor.evictReadySent = false
	}
}

// finishShutdownStop 回复关服请求方并停止当前角色。
func (actor *PlayerActor) finishShutdownStop(ctx tree.Context, err error) {
	request := actor.shutdownRequest
	actor.shutdownRequest = nil
	actor.shutdownStop = false
	if request != nil {
		request.Respond(nil, err)
	}
	ctx.Stop()
}

// notifyEvictReady 通知网关当前玩家角色已完成存盘，可以安全回收。
func (actor *PlayerActor) notifyEvictReady(ctx tree.Context) {
	if actor.online || actor.evictReadySent || actor.gatePID.ID == 0 {
		return
	}
	actor.evictReadySent = true
	if !ctx.Send(actor.gatePID, &ipb.PlayerActorEvictReady{UID: actor.UID, PID: ctx.Self()}) {
		actor.evictReadySent = false
		tlog.Log.Error("[play/PlayerActor.notifyEvictReady] 通知网关回收玩家 Actor 失败, pid=%v", actor.UID)
	}
}

// push 通过所属网关转发客户端响应。
func (actor *PlayerActor) push(ctx tree.Context, uid uint64, msg interface{}) bool {
	gatePID := actor.gatePID
	if gatePID.ID == 0 {
		gatePID, _ = ctx.Lookup(def.GameGate)
	}
	if gatePID.ID == 0 {
		return false
	}
	return ctx.Send(gatePID, &ipb.PushToMsg{UID: uid, Msg: msg})
}

// EventFunc 是玩家业务事件的监听器签名。
type EventFunc func(actor *PlayerActor, arg *comm.Arg)

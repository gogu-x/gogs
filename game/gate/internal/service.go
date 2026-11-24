package internal

import (
	"fmt"
	"net"
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/game/play"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/grpc"
)

const (
	// playerRetryTimerType 是重投排队业务和重试停止离线玩家 Actor 的定时器类型。
	playerRetryTimerType = timer.TimerType(1000002)
	// playerRetryInterval 是 playerRetryTimerType 的触发间隔。
	playerRetryInterval = time.Second
	// maxQueuedDeliveriesPerPlayer 是单个玩家在网关排队等待的业务投递上限。
	maxQueuedDeliveriesPerPlayer = 512
)

type playerActorState struct {
	pid              tree.PID
	evicting         bool
	queuedDeliveries []interface{} // 玩家 Actor 暂时无法接收（正忙、回收中或有更早的排队）时保留的业务投递，按到达顺序重投。
}

// queueDelivery 将暂时无法交给玩家 Actor 的业务投递加入排队；队列已满时返回 false。
func (state *playerActorState) queueDelivery(message interface{}) bool {
	if state == nil || len(state.queuedDeliveries) >= maxQueuedDeliveriesPerPlayer {
		return false
	}
	state.queuedDeliveries = append(state.queuedDeliveries, message)
	return true
}

// takeQueuedDeliveries 取出并清空该玩家全部排队的业务投递。
func (state *playerActorState) takeQueuedDeliveries() []interface{} {
	if state == nil {
		return nil
	}
	pending := state.queuedDeliveries
	state.queuedDeliveries = nil
	return pending
}

// GateActor 管理网关会话、玩家角色目录和消息重试。
type GateActor struct {
	grpcServer        *grpc.Server
	router            tree.Router
	sessions          map[uint64]session
	playerActors      map[uint64]*playerActorState
	pendingOpen       map[uint64]*openSession
	bannedUIDs        map[uint64]struct{}
	nextGen           uint64
	playerRetryWheel  *timer.TimeWheel
	playerRetryTimer  *timer.WheelTimer
	serverOpened      bool
	serverOpenedEvent *sspb.Event
	shuttingDown      bool
	shutdownWaiter    *tree.Envelope
	shutdownRemaining int
	shutdownErr       error
}

// NewGateActor 创建网关角色及玩家角色目录。
func NewGateActor() *GateActor {
	return &GateActor{
		sessions:     make(map[uint64]session),
		playerActors: make(map[uint64]*playerActorState),
		pendingOpen:  make(map[uint64]*openSession),
		bannedUIDs:   make(map[uint64]struct{}),
	}
}

// Name 返回网关 Actor 的注册名称。
func (g *GateActor) Name() string { return def.GameGate }

// MailboxSize 返回网关消息队列容量，用于容纳集中到达的服务器推送。
func (g *GateActor) MailboxSize() int { return 8 * 1024 }

// OnInit 注册网关消息处理方法并初始化会话目录。
func (g *GateActor) OnInit(ctx tree.Context) {
	g.router.Register(&openSession{}, g.onOpenSession)
	g.router.Register(&closeSession{}, g.onCloseSession)
	g.router.Register(&ipb.CloseSession{}, g.onPlayerCloseSession)
	g.router.Register(&ipb.DeliverToPlayer{}, g.onDeliverToPlayer)
	g.router.Register(&ipb.PlayerActorEvictReady{}, g.onPlayerActorEvictReady)
	g.router.Register(&ipb.PlayerActorStopped{}, g.onPlayerActorStopped)
	g.router.Register(&ipb.PlayerLoadFailed{}, g.onPlayerLoadFailed)
	g.router.Register(&sspb.Event{}, g.onServerEvent)
	g.router.Register(&ipb.BanUID{}, g.onBanUID)
	g.router.Register(&ipb.UnbanUID{}, g.onUnbanUID)

	g.playerRetryWheel = timer.NewTimeWheel(32, ctx.Self())
	g.playerRetryWheel.Register(playerRetryTimerType, func(_ interface{}) {
		g.onPlayerRetryTimer()
	})

	lis, err := net.Listen("tcp", conf.GameAddr())
	if err != nil || lis == nil {
		tlog.Log.Error("[gate/GateActor.OnInit] 网关监听启动失败, addr=%v err=%v", conf.GameAddr(), err)
		return
	}

	g.grpcServer = grpc.NewServer()
	pb_gateway.RegisterGatewayServer(g.grpcServer, &gatewayService{
		actorPID: ctx.Self(),
		codec:    codec.ProtoCodec,
	})

	go func() {
		tlog.Log.Info("[gate/GateActor.OnInit] gRPC 网关开始监听, addr=%v", conf.GameAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			tlog.Log.Error("[gate/GateActor.OnInit] gRPC 网关已停止, err=%v", err)
		}
	}()
}

// HandleMessage 将网关消息分发给已注册的处理方法。
func (g *GateActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

// OnStop 关闭网关接入并停止相关定时器。
func (g *GateActor) OnStop(ctx tree.Context) {
	if g.playerRetryWheel != nil {
		g.playerRetryWheel.Stop()
	}
	if g.playerRetryTimer != nil {
		g.playerRetryTimer.Stop()
		g.playerRetryTimer = nil
	}
	for uid, current := range g.sessions {
		delete(g.sessions, uid)
		ctx.Send(current.pid, &stopAgent{})
		ctx.Send(current.playerPID, &ipb.SessionClosed{UID: uid, Generation: current.generation})
	}
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
}

// onServerEvent 根据服务器生命周期事件启动开服或关服处理。
func (g *GateActor) onServerEvent(ctx tree.Context, message interface{}) {
	serverEvent, ok := message.(*sspb.Event)
	if !ok || serverEvent == nil || !gameevent.IsServerLifecycle(serverEvent) {
		err := fmt.Errorf("unsupported server lifecycle event %T", message)
		tlog.Log.Error("[gate/GateActor.onServerEvent] 收到不支持的生命周期消息, type=%T err=%v", message, err)
		ctx.Response(nil, err)
		return
	}
	switch {
	case gameevent.IsServerOpened(serverEvent):
		g.onServerOpened(ctx, serverEvent)
	case gameevent.IsServerClosing(serverEvent):
		g.onServerClosing(ctx, serverEvent)
	default:
		eventName := serverEvent.GetName()
		err := fmt.Errorf("unsupported server lifecycle event %q", eventName)
		tlog.Log.Error("[gate/GateActor.onServerEvent] 收到不支持的生命周期事件, event=%v err=%v", eventName, err)
		ctx.Response(nil, err)
	}
}

// onServerOpened 处理服务器开服事件并恢复网关接入。
func (g *GateActor) onServerOpened(ctx tree.Context, message *sspb.Event) {
	if g.shuttingDown {
		ctx.Response(nil, fmt.Errorf("game gateway is shutting down"))
		return
	}
	if g.grpcServer == nil {
		ctx.Response(nil, fmt.Errorf("game gateway listener is unavailable"))
		return
	}
	if g.serverOpened {
		ctx.Response(nil, nil)
		return
	}
	g.serverOpenedEvent = gameevent.Clone(message)
	g.serverOpened = true
	for uid, state := range g.playerActors {
		if state == nil {
			continue
		}
		if state.pid.ID != 0 && !ctx.Send(state.pid, gameevent.Clone(message)) {
			tlog.Log.Error("[gate/GateActor.onServerOpened] 通知玩家 Actor 开服事件失败, pid=%v playerPID=%v", uid, state.pid)
		}
	}
	tlog.Log.Info("[gate/GateActor.onServerOpened] 网关已接收开服事件, playerActors=%v", len(g.playerActors))
	ctx.Response(nil, nil)
}

// onServerClosing 将关服事件交给网关生命周期协调流程。
func (g *GateActor) onServerClosing(ctx tree.Context, _ *sspb.Event) {
	g.beginServerShutdown(ctx)
}

// spawnPlayerActor 创建玩家 Actor 并传入需要优先处理的初始消息。
func (g *GateActor) spawnPlayerActor(ctx tree.Context, uid uint64, messages []interface{}) tree.PID {
	var actor tree.Actor
	if g.serverOpened && !g.shuttingDown {
		actor = play.NewPlayerActorWithMessages(uid, g.prependServerOpened(messages))
	} else if len(messages) > 0 {
		actor = play.NewPlayerActorWithMessages(uid, messages)
	} else {
		actor = play.NewPlayerActor(uid)
	}
	return tree.SpawnOne(actor)
}

// prependServerOpened 将开服事件放到玩家 Actor 的初始消息队列前部。
func (g *GateActor) prependServerOpened(messages []interface{}) []interface{} {
	initialMessages := make([]interface{}, 0, len(messages)+1)
	serverOpenedEvent := g.serverOpenedEvent
	if serverOpenedEvent == nil {
		serverOpenedEvent = gameevent.NewServerOpened()
	}
	initialMessages = append(initialMessages, gameevent.Clone(serverOpenedEvent))
	initialMessages = append(initialMessages, messages...)
	return initialMessages
}

// onOpenSession 验证登录流并为玩家创建或复用会话。
func (g *GateActor) onOpenSession(ctx tree.Context, msg interface{}) {
	if g.shuttingDown {
		ctx.Response(nil, fmt.Errorf("game gateway is shutting down"))
		return
	}
	if !g.serverOpened {
		ctx.Response(nil, fmt.Errorf("game gateway has not completed startup"))
		return
	}
	open, ok := msg.(*openSession)
	if !ok || open == nil {
		ctx.Response(nil, fmt.Errorf("invalid gateway open session"))
		return
	}
	open.request = ctx.RequestEnvelope()
	if openSessionCanceled(open) {
		ctx.Response(nil, fmt.Errorf("gateway stream is already closed"))
		return
	}
	firstMsg, ok := open.msg.(*pb_auth.LoginGameReq)
	if !ok {
		ctx.Response(nil, fmt.Errorf("first stream message must be LoginGameReq, got %T", open.msg))
		return
	}
	uid := firstMsg.GetUID()
	if uid == 0 {
		ctx.Response(nil, fmt.Errorf("login uid must be non-zero"))
		return
	}
	if _, banned := g.bannedUIDs[uid]; banned {
		ctx.Response(nil, fmt.Errorf("uid %d is banned", uid))
		return
	}
	if state := g.playerActors[uid]; state != nil && state.evicting {
		if open.request == nil {
			ctx.Response(nil, fmt.Errorf("cannot defer gateway session without a request envelope"))
			return
		}
		if _, exists := g.pendingOpen[uid]; exists {
			ctx.Response(nil, fmt.Errorf("uid %d already has a pending gateway session", uid))
			return
		}
		g.pendingOpen[uid] = open
		tlog.Log.Info("[gate/GateActor.onOpenSession] 玩家 Actor 正在回收，延后建立会话, pid=%v playerPID=%v", uid, state.pid)
		return
	}
	opened, err := g.createSession(ctx, open)
	if err == nil && g.rejectCanceledOpen(ctx, open, opened) {
		opened = nil
		err = fmt.Errorf("gateway stream closed while opening session")
	}
	if err != nil {
		ctx.Response(nil, err)
		return
	}
	ctx.Response(opened, nil)
}

// createSession 绑定玩家 Actor、创建网关代理并启动玩家会话。
func (g *GateActor) createSession(ctx tree.Context, open *openSession) (*openedSession, error) {
	if openSessionCanceled(open) {
		return nil, fmt.Errorf("gateway stream is already closed")
	}
	firstMsg, ok := open.msg.(*pb_auth.LoginGameReq)
	if !ok || firstMsg == nil {
		return nil, fmt.Errorf("first stream message must be LoginGameReq")
	}
	uid := firstMsg.GetUID()
	state := g.playerActors[uid]
	if state == nil {
		state = &playerActorState{}
		g.playerActors[uid] = state
	}
	if state.pid.ID == 0 {
		state.pid = g.spawnPlayerActor(ctx, uid, nil)
		tlog.Log.Info("[gate/GateActor.createSession] 已创建玩家 Actor, pid=%v playerPID=%v", uid, state.pid)
	}
	if state.evicting {
		return nil, fmt.Errorf("player actor %d is stopping", uid)
	}
	g.nextGen++
	generation := g.nextGen
	// 先创建会话代理：同名注册会覆盖旧会话，玩家 Actor 绑定新会话后按名称推送即到达新代理。
	agentPID := tree.SpawnOne(newGsAgent(uid, generation, open.stream, ctx.Self(), state.pid))
	if !ctx.Send(state.pid, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
		// 玩家 Actor 已失效：代理持有旧玩家 PID，需与新玩家 Actor 一起重建。
		// 旧代理退出时框架只删除 PID 一致的名称登记，不影响新代理。
		ctx.Send(agentPID, &stopAgent{})
		state.pid = g.spawnPlayerActor(ctx, uid, nil)
		tlog.Log.Info("[gate/GateActor.createSession] 玩家 Actor 已失效，创建新 Actor, pid=%v playerPID=%v", uid, state.pid)
		agentPID = tree.SpawnOne(newGsAgent(uid, generation, open.stream, ctx.Self(), state.pid))
		if !ctx.Send(state.pid, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
			ctx.Send(agentPID, &stopAgent{})
			delete(g.playerActors, uid)
			return nil, fmt.Errorf("could not bind gateway session to player actor")
		}
	}
	if previous, exists := g.sessions[uid]; exists {
		ctx.Send(previous.pid, &stopAgent{})
	}
	g.sessions[uid] = session{
		pid:        agentPID,
		playerPID:  state.pid,
		generation: generation,
		uid:        uid,
	}
	if !ctx.Send(agentPID, &inboundFrame{msg: open.msg}) {
		delete(g.sessions, uid)
		ctx.Send(agentPID, &stopAgent{})
		ctx.Send(state.pid, &ipb.SessionClosed{UID: uid, Generation: generation})
		return nil, fmt.Errorf("could not send initial login message to gateway agent")
	}
	tlog.Log.Info("[gate/GateActor.createSession] 玩家网关会话已建立, pid=%v generation=%v playerPID=%v",
		uid, generation, state.pid)
	return &openedSession{pid: agentPID, uid: uid, generation: generation}, nil
}

// onCloseSession 关闭指定版本的玩家网关会话。
func (g *GateActor) onCloseSession(ctx tree.Context, msg interface{}) {
	closed, ok := msg.(*closeSession)
	if !ok || closed == nil {
		return
	}
	current, ok := g.sessions[closed.uid]
	if !ok || current.pid != closed.pid || current.generation != closed.generation {
		return
	}
	delete(g.sessions, closed.uid)
	ctx.Send(current.pid, &stopAgent{})
	g.notifySessionClosed(ctx, closed.uid, current.playerPID, current.generation)
	tlog.Log.Info("[gate/GateActor.onCloseSession] 网关会话已关闭, pid=%v reason=%v", closed.uid, closed.reason)
}

// onPlayerCloseSession 处理推送方发起的会话关闭请求，例如直接推送时代理出站队列已满。
//
// 只关闭请求中指定的代理；该代理已被新会话替换时忽略，避免误关新连接。
func (g *GateActor) onPlayerCloseSession(ctx tree.Context, msg interface{}) {
	request, ok := msg.(*ipb.CloseSession)
	if !ok || request == nil {
		return
	}
	current, ok := g.sessions[request.UID]
	if !ok || current.pid != request.AgentPID {
		return
	}
	g.closeCurrent(ctx, request.UID, current, request.Reason)
}

// onDeliverToPlayer 是服务端修改或查询玩家数据的统一入口：按离线规则决定是否受理，
// 定位在线玩家、复用驻留的 Actor 或唤醒离线玩家；以请求方式到达时保留回复目标，
// 由玩家 Actor 把实际执行结果回给原调用方。
func (g *GateActor) onDeliverToPlayer(ctx tree.Context, msg interface{}) {
	if g.shuttingDown {
		ctx.Response(nil, fmt.Errorf("game gateway is shutting down"))
		tlog.Log.Error("[gate/GateActor.onDeliverToPlayer] 服务关停期间拒绝玩家业务投递, type=%T", msg)
		return
	}
	request, ok := msg.(*ipb.DeliverToPlayer)
	if !ok || request == nil || request.GetUID() == 0 || request.GetPayload() == nil {
		ctx.Response(nil, fmt.Errorf("invalid player delivery"))
		tlog.Log.Error("[gate/GateActor.onDeliverToPlayer] 玩家业务投递无效, type=%T", msg)
		return
	}
	uid := request.GetUID()
	if _, online := g.sessions[uid]; request.GetRequireOnline() && !online {
		ctx.Response(nil, fmt.Errorf("player %d is offline", uid))
		return
	}
	// 需要结果的请求把回复目标绑在载荷上，排队、唤醒、重投期间一路携带，由玩家 Actor 回复。
	var delivery interface{} = request.GetPayload()
	if reply := ctx.RequestEnvelope(); reply != nil {
		delivery = ipb.WrapWithReply(delivery, reply)
	}
	state := g.playerActors[uid]
	if state == nil {
		state = &playerActorState{}
		g.playerActors[uid] = state
	}
	if state.pid.ID == 0 {
		state.pid = g.spawnPlayerActor(ctx, uid, []interface{}{delivery})
		tlog.Log.Info("[gate/GateActor.onDeliverToPlayer] 为离线玩家唤醒 Actor, pid=%v playerPID=%v type=%T", uid, state.pid, request.GetPayload())
		return
	}
	// 已有排队投递时新投递排在后面，避免较新的业务先于旧业务生效。
	if !state.evicting && !state.hasQueuedDeliveries() && ctx.TrySend(state.pid, delivery) {
		return
	}
	if !state.queueDelivery(delivery) {
		ipb.RejectDelivery(delivery, fmt.Errorf("player %d delivery queue is full", uid))
		tlog.Log.Error("[gate/GateActor.onDeliverToPlayer] 玩家业务投递排队已满, pid=%v type=%T", uid, request.GetPayload())
		return
	}
	tlog.Log.Info("[gate/GateActor.onDeliverToPlayer] 玩家 Actor 正忙或正在回收，业务投递已排队, pid=%v type=%T", uid, request.GetPayload())
	g.schedulePlayerRetry()
}

// onPlayerActorEvictReady 在离线玩家 Actor 可以回收时请求其停止。
func (g *GateActor) onPlayerActorEvictReady(ctx tree.Context, msg interface{}) {
	ready, ok := msg.(*ipb.PlayerActorEvictReady)
	if !ok || ready == nil {
		return
	}
	state := g.playerActors[ready.UID]
	if state == nil || state.pid != ready.PID {
		return
	}
	if _, online := g.sessions[ready.UID]; online {
		return
	}
	if state.evicting || g.shuttingDown {
		return
	}
	if state.hasQueuedDeliveries() {
		g.resendQueuedDeliveries(state)
		g.schedulePlayerRetry()
		return
	}
	state.evicting = true
	if !ctx.Send(state.pid, &ipb.StopPlayerActor{UID: ready.UID}) {
		tlog.Log.Error("[gate/GateActor.onPlayerActorEvictReady] 请求停止离线玩家 Actor 失败，将由重试定时器重试, pid=%v playerPID=%v", ready.UID, ready.PID)
	}
	g.schedulePlayerRetry()
}

// onPlayerActorStopped 清理已停止的玩家 Actor 并恢复等待中的会话或消息。
func (g *GateActor) onPlayerActorStopped(ctx tree.Context, msg interface{}) {
	stopped, ok := msg.(*ipb.PlayerActorStopped)
	if !ok || stopped == nil {
		return
	}
	state := g.playerActors[stopped.UID]
	if state == nil || state.pid != stopped.PID {
		return
	}
	if g.shuttingDown && state.evicting {
		if state.hasQueuedDeliveries() {
			state.evicting = false
			state.pid = g.spawnPlayerActor(ctx, stopped.UID, nil)
			g.stopPlayerForShutdown(ctx, stopped.UID, state)
		} else {
			delete(g.playerActors, stopped.UID)
			g.onPlayerShutdownComplete(ctx, stopped.UID, nil)
		}
		return
	}
	queuedDeliveries := state.takeQueuedDeliveries()
	state.pid = tree.PID{}
	state.evicting = false
	if open, exists := g.pendingOpen[stopped.UID]; exists {
		delete(g.pendingOpen, stopped.UID)
		if g.shuttingDown {
			open.request.Respond(nil, fmt.Errorf("game gateway is shutting down"))
			ipb.RejectDeliveries(queuedDeliveries, fmt.Errorf("game gateway is shutting down"))
			delete(g.playerActors, stopped.UID)
			return
		}
		if openSessionCanceled(open) {
			open.request.Respond(nil, fmt.Errorf("gateway stream closed before player actor was ready"))
			if len(queuedDeliveries) > 0 {
				state.pid = g.spawnPlayerActor(ctx, stopped.UID, queuedDeliveries)
			}
			return
		}
		if _, banned := g.bannedUIDs[stopped.UID]; banned {
			open.request.Respond(nil, fmt.Errorf("uid %d is banned", stopped.UID))
			if len(queuedDeliveries) > 0 {
				state.pid = g.spawnPlayerActor(ctx, stopped.UID, queuedDeliveries)
			}
			return
		}
		state.pid = g.spawnPlayerActor(ctx, stopped.UID, queuedDeliveries)
		opened, err := g.createSession(ctx, open)
		if err == nil && g.rejectCanceledOpen(ctx, open, opened) {
			opened = nil
			err = fmt.Errorf("gateway stream closed while opening session")
		}
		open.request.Respond(opened, err)
		return
	}
	if g.shuttingDown {
		ipb.RejectDeliveries(queuedDeliveries, fmt.Errorf("game gateway is shutting down"))
		delete(g.playerActors, stopped.UID)
		return
	}
	if len(queuedDeliveries) > 0 {
		state.pid = g.spawnPlayerActor(ctx, stopped.UID, queuedDeliveries)
		tlog.Log.Info("[gate/GateActor.onPlayerActorStopped] 已重建玩家 Actor 并恢复排队业务投递, pid=%v playerPID=%v count=%v", stopped.UID, state.pid, len(queuedDeliveries))
	}
	if state.pid.ID == 0 {
		delete(g.playerActors, stopped.UID)
	}
}

// beginServerShutdown 停止网关接入，并协调玩家角色完成关服流程。
func (g *GateActor) beginServerShutdown(ctx tree.Context) {
	if g.shuttingDown {
		ctx.Response(nil, fmt.Errorf("server shutdown is already in progress"))
		return
	}
	g.shuttingDown = true
	g.shutdownWaiter = ctx.RequestEnvelope()
	g.shutdownRemaining = 0
	g.shutdownErr = nil
	if g.playerRetryWheel != nil {
		g.playerRetryWheel.Stop()
	}
	if g.playerRetryTimer != nil {
		g.playerRetryTimer.Stop()
		g.playerRetryTimer = nil
	}
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
	for uid, open := range g.pendingOpen {
		delete(g.pendingOpen, uid)
		if open.request != nil {
			open.request.Respond(nil, fmt.Errorf("game gateway is shutting down"))
		}
	}
	for uid, current := range g.sessions {
		delete(g.sessions, uid)
		ctx.Send(current.pid, &stopAgent{})
		g.notifySessionClosed(ctx, uid, current.playerPID, current.generation)
	}
	for uid, state := range g.playerActors {
		if state.pid.ID == 0 {
			continue
		}
		g.shutdownRemaining++
		if !state.evicting {
			g.stopPlayerForShutdown(ctx, uid, state)
		}
	}
	if g.shutdownRemaining == 0 {
		g.finishServerShutdown()
	}
	tlog.Log.Info("[gate/GateActor.beginServerShutdown] 已停止网关接入并等待玩家角色完成关服, count=%v", g.shutdownRemaining)
}

// stopPlayerForShutdown 将关服前已接收的系统消息与最终存盘请求按顺序交给玩家。
func (g *GateActor) stopPlayerForShutdown(ctx tree.Context, uid uint64, state *playerActorState) {
	pending := state.takeQueuedDeliveries()
	stop := &ipb.StopPlayerActor{UID: uid, Shutdown: true, QueuedDeliveries: append(pending, gameevent.NewServerClosing())}
	ctx.RequestCallback(state.pid, stop, func(callbackCtx tree.Context, _ interface{}, err error) {
		if err != nil {
			ipb.RejectDeliveries(pending, err)
		}
		g.onPlayerShutdownComplete(callbackCtx, uid, err)
	})
}

// onPlayerShutdownComplete 记录玩家角色的关服结果并更新等待数量。
func (g *GateActor) onPlayerShutdownComplete(_ tree.Context, uid uint64, err error) {
	if err != nil && g.shutdownErr == nil {
		g.shutdownErr = err
		tlog.Log.Error("[gate/GateActor.onPlayerShutdownComplete] 玩家角色关服处理失败, pid=%v err=%v", uid, err)
	}
	if g.shutdownRemaining > 0 {
		g.shutdownRemaining--
	}
	if g.shutdownRemaining == 0 {
		g.finishServerShutdown()
	}
}

// finishServerShutdown 在所有玩家角色完成关服后回复生命周期请求。
func (g *GateActor) finishServerShutdown() {
	waiter := g.shutdownWaiter
	g.shutdownWaiter = nil
	if waiter != nil {
		waiter.Respond(nil, g.shutdownErr)
	}
}

// onPlayerLoadFailed 关闭数据加载失败玩家的在线会话。
func (g *GateActor) onPlayerLoadFailed(ctx tree.Context, msg interface{}) {
	failed, ok := msg.(*ipb.PlayerLoadFailed)
	if !ok || failed == nil {
		return
	}
	state := g.playerActors[failed.UID]
	if state == nil || state.pid != failed.PID {
		return
	}
	if current, online := g.sessions[failed.UID]; online {
		g.closeCurrent(ctx, failed.UID, current, "player data load failed")
	}
	tlog.Log.Error("[gate/GateActor.onPlayerLoadFailed] 玩家数据加载失败，已关闭会话, pid=%v reason=%v", failed.UID, failed.Reason)
}

// onBanUID 封禁玩家并关闭其当前网关会话。
func (g *GateActor) onBanUID(ctx tree.Context, msg interface{}) {
	ban, ok := msg.(*ipb.BanUID)
	if !ok || ban == nil || ban.UID == 0 {
		return
	}
	g.bannedUIDs[ban.UID] = struct{}{}
	if current, exists := g.sessions[ban.UID]; exists {
		g.closeCurrent(ctx, ban.UID, current, "user is banned")
	}
}

// onUnbanUID 从当前网关的内存封禁列表中移除玩家。
func (g *GateActor) onUnbanUID(_ tree.Context, msg interface{}) {
	unban, ok := msg.(*ipb.UnbanUID)
	if !ok || unban == nil {
		return
	}
	delete(g.bannedUIDs, unban.UID)
}

// closeCurrent 从网关会话目录中移除玩家并通知玩家 Actor 断线。
func (g *GateActor) closeCurrent(
	ctx tree.Context,
	uid uint64,
	current session,
	reason string,
) {
	delete(g.sessions, uid)
	ctx.Send(current.pid, &stopAgent{})
	g.notifySessionClosed(ctx, uid, current.playerPID, current.generation)
	tlog.Log.Info("[gate/GateActor.closeCurrent] 网关会话已关闭, pid=%v reason=%v", uid, reason)
}

// rejectCanceledOpen 检查登录流是否已取消并关闭刚创建的会话。
func (g *GateActor) rejectCanceledOpen(ctx tree.Context, open *openSession, opened *openedSession) bool {
	if !openSessionCanceled(open) {
		return false
	}
	if opened != nil {
		if current, ok := g.sessions[opened.uid]; ok && current.generation == opened.generation {
			g.closeCurrent(ctx, opened.uid, current, "gateway stream closed while opening session")
		}
	}
	return true
}

// notifySessionClosed 将网关会话关闭消息发送给玩家 Actor。
func (g *GateActor) notifySessionClosed(
	ctx tree.Context,
	uid uint64,
	playerPID tree.PID,
	generation uint64,
) {
	if playerPID.ID != 0 && !ctx.Send(playerPID, &ipb.SessionClosed{UID: uid, Generation: generation}) {
		tlog.Log.Error("[gate/GateActor.notifySessionClosed] 通知玩家 Actor 断线失败, pid=%v playerPID=%v", uid, playerPID)
	}
}

// onPlayerRetryTimer 重投排队的业务投递，并重试停止已可回收的离线玩家 Actor。
func (g *GateActor) onPlayerRetryTimer() {
	g.playerRetryTimer = nil
	if g.shuttingDown {
		return
	}
	for uid, state := range g.playerActors {
		if state == nil {
			continue
		}
		if state.evicting && state.pid.ID != 0 {
			if !tree.TrySend(state.pid, &ipb.StopPlayerActor{UID: uid}) {
				tlog.Log.Error("[gate/GateActor.onPlayerRetryTimer] 重试停止离线玩家 Actor 失败, pid=%v playerPID=%v", uid, state.pid)
			}
			continue
		}
		if state.hasQueuedDeliveries() {
			g.resendQueuedDeliveries(state)
		}
	}
	g.schedulePlayerRetry()
}

// resendQueuedDeliveries 按到达顺序把排队的业务投递交给玩家 Actor，遇到对方队列已满即停止并保留剩余部分。
func (g *GateActor) resendQueuedDeliveries(state *playerActorState) {
	if state == nil || state.evicting || state.pid.ID == 0 {
		return
	}
	queued := state.takeQueuedDeliveries()
	for index, delivery := range queued {
		if !tree.TrySend(state.pid, delivery) {
			// 取出的数量不超过上限，原样放回不会溢出。
			state.queuedDeliveries = queued[index:]
			return
		}
	}
}

// schedulePlayerRetry 在仍有排队投递或待停止的 Actor 时安排下一次重试。
func (g *GateActor) schedulePlayerRetry() {
	if g.playerRetryWheel == nil || g.playerRetryTimer != nil || g.shuttingDown {
		return
	}
	if !g.hasPlayerRetryWork() {
		return
	}
	g.playerRetryTimer = g.playerRetryWheel.After(playerRetryTimerType, playerRetryInterval, nil)
	if g.playerRetryTimer == nil {
		tlog.Log.Error("[gate/GateActor.schedulePlayerRetry] 创建玩家系统消息重试定时器失败")
	}
}

// hasPlayerRetryWork 检查是否还有排队投递或待停止的玩家 Actor。
func (g *GateActor) hasPlayerRetryWork() bool {
	for _, state := range g.playerActors {
		if state == nil || state.pid.ID == 0 {
			continue
		}
		if state.evicting || state.hasQueuedDeliveries() {
			return true
		}
	}
	return false
}

// hasQueuedDeliveries 返回该玩家是否还有排队等待的业务投递。
func (state *playerActorState) hasQueuedDeliveries() bool {
	return state != nil && len(state.queuedDeliveries) > 0
}

// validateFrame 检查客户端网关帧及其协议负载是否有效。
func validateFrame(frame *pb_gateway.Frame) error {
	if frame == nil {
		return fmt.Errorf("empty gateway frame")
	}
	if len(frame.GetPayload()) == 0 {
		return fmt.Errorf("empty gateway payload")
	}
	return nil
}

// openSessionCanceled 检查登录流或其上下文是否已经关闭。
func openSessionCanceled(open *openSession) bool {
	return open == nil || open.stream == nil || open.stream.Context().Err() != nil
}

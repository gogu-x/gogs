package internal

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/activity"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/game/play"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/timer"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/grpc"
)

const (
	playerMessageRetryTimerType = timer.TimerType(1000002)
	playerMessageRetryInterval  = time.Second
	maxPendingPlayerMessages    = 512
)

type playerActorState struct {
	pid         tree.PID
	activityPID tree.PID
	evicting    bool
	pending     []interface{}
}

func (state *playerActorState) AddPending(message interface{}) bool {
	if state == nil || len(state.pending) >= maxPendingPlayerMessages {
		return false
	}
	state.pending = append(state.pending, message)
	return true
}

func (state *playerActorState) TakePending() []interface{} {
	if state == nil {
		return nil
	}
	pending := state.pending
	state.pending = nil
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
	system            *tree.Tree
	messageRetryWheel *timer.TimeWheel
	messageRetryTimer *timer.WheelTimer
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

func (g *GateActor) Name() string { return def.GameGate }

// MailboxSize 返回网关消息队列容量，用于容纳集中到达的服务器推送。
func (g *GateActor) MailboxSize() int { return 8 * 1024 }

func (g *GateActor) OnInit(ctx tree.Context) {
	g.system = ctx.System()
	g.router.Register(&openSession{}, g.onOpenSession)
	g.router.Register(&closeSession{}, g.onCloseSession)
	g.router.Register(&ipb.PushToMsg{}, g.onPushToMsg)
	g.router.Register(&ipb.PlayerMessage{}, g.onPlayerMessage)
	g.router.Register(&ipb.PlayerActorEvictReady{}, g.onPlayerActorEvictReady)
	g.router.Register(&ipb.PlayerActorStopped{}, g.onPlayerActorStopped)
	g.router.Register(&ipb.PlayerLoadFailed{}, g.onPlayerLoadFailed)
	g.router.Register(&sspb.Event{}, g.onServerEvent)
	g.router.Register(&ipb.BanUID{}, g.onBanUID)
	g.router.Register(&ipb.UnbanUID{}, g.onUnbanUID)

	g.messageRetryWheel = timer.NewTimeWheel(32, ctx.Self(), ctx.System())
	g.messageRetryWheel.Register(playerMessageRetryTimerType, func(_ interface{}) {
		g.onPlayerMessageRetryTimer()
	})

	lis, err := net.Listen("tcp", conf.GameAddr())
	if err != nil || lis == nil {
		tlog.Log.Error("[gate/GateActor.OnInit] 网关监听启动失败, addr=%v err=%v", conf.GameAddr(), err)
		return
	}

	g.grpcServer = grpc.NewServer()
	pb_gateway.RegisterGatewayServer(g.grpcServer, &gatewayService{
		actorPID: ctx.Self(),
		system:   ctx.System(),
		codec:    codec.ProtoCodec,
	})

	go func() {
		tlog.Log.Info("[gate/GateActor.OnInit] gRPC 网关开始监听, addr=%v", conf.GameAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			tlog.Log.Error("[gate/GateActor.OnInit] gRPC 网关已停止, err=%v", err)
		}
	}()

	if err := cluster.Register(
		fmt.Sprintf("%d", conf.ServerID),
		fmt.Sprintf("%d", os.Getpid()),
		conf.GameAddr(),
	); err != nil {
		tlog.Log.Error("[gate/GateActor.OnInit] 网关集群注册失败, server=%v err=%v", conf.ServerID, err)
	} else {
		tlog.Log.Info("[gate/GateActor.OnInit] 网关已注册到集群, server=%v addr=%v", conf.ServerID, conf.GameAddr())
	}
}

func (g *GateActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GateActor) OnStop(ctx tree.Context) {
	if g.messageRetryWheel != nil {
		g.messageRetryWheel.Stop()
	}
	if g.messageRetryTimer != nil {
		g.messageRetryTimer.Stop()
		g.messageRetryTimer = nil
	}
	for uid, current := range g.sessions {
		delete(g.sessions, uid)
		ctx.Send(current.pid, &stopAgent{})
		ctx.Send(current.playerPID, &ipb.SessionClosed{UID: uid, Generation: current.generation})
		ctx.Send(current.activityPID, &ipb.SessionClosed{UID: uid, Generation: current.generation})
	}
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}
}

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
		if state.activityPID.ID != 0 && !ctx.Send(state.activityPID, gameevent.Clone(message)) {
			tlog.Log.Error("[gate/GateActor.onServerOpened] 通知玩家活动 Actor 开服事件失败, pid=%v activityPID=%v", uid, state.activityPID)
		}
	}
	tlog.Log.Info("[gate/GateActor.onServerOpened] 网关已接收开服事件, playerActors=%v", len(g.playerActors))
	ctx.Response(nil, nil)
}

// onServerClosing 将关服事件交给网关生命周期协调流程。
func (g *GateActor) onServerClosing(ctx tree.Context, message *sspb.Event) {
	g.beginServerShutdown(ctx, message)
}

func (g *GateActor) spawnPlayerActor(ctx tree.Context, uid uint64, messages []interface{}) tree.PID {
	var actor tree.Actor
	if g.serverOpened && !g.shuttingDown {
		actor = play.NewPlayerActorWithMessages(uid, g.prependServerOpened(messages))
	} else if len(messages) > 0 {
		actor = play.NewPlayerActorWithMessages(uid, messages)
	} else {
		actor = play.NewPlayerActor(uid)
	}
	return ctx.System().SpawnOne(actor)
}

func (g *GateActor) spawnPlayerActivityActor(ctx tree.Context, uid uint64, messages []interface{}) tree.PID {
	var actor tree.Actor
	if g.serverOpened && !g.shuttingDown {
		actor = activity.NewPlayerActivityActorWithMessages(uid, g.prependServerOpened(messages))
	} else if len(messages) > 0 {
		actor = activity.NewPlayerActivityActorWithMessages(uid, messages)
	} else {
		actor = activity.NewPlayerActivityActor(uid)
	}
	return ctx.System().SpawnOne(actor)
}

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
	if state.activityPID.ID == 0 {
		state.activityPID = g.spawnPlayerActivityActor(ctx, uid, nil)
		tlog.Log.Info("[gate/GateActor.createSession] 已创建玩家活动 Actor, pid=%v activityPID=%v", uid, state.activityPID)
	}
	if state.evicting {
		return nil, fmt.Errorf("player actor %d is stopping", uid)
	}
	g.nextGen++
	generation := g.nextGen
	if !ctx.Send(state.pid, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
		state.pid = g.spawnPlayerActor(ctx, uid, nil)
		tlog.Log.Info("[gate/GateActor.createSession] 玩家 Actor 已失效，创建新 Actor, pid=%v playerPID=%v", uid, state.pid)
		if !ctx.Send(state.pid, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
			delete(g.playerActors, uid)
			return nil, fmt.Errorf("could not bind gateway session to player actor")
		}
	}
	if !ctx.Send(state.activityPID, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
		state.activityPID = g.spawnPlayerActivityActor(ctx, uid, nil)
		if !ctx.Send(state.activityPID, &ipb.PlayerSessionOpened{UID: uid, Generation: generation}) {
			return nil, fmt.Errorf("could not bind gateway session to player activity actor")
		}
	}
	if previous, exists := g.sessions[uid]; exists {
		ctx.Send(previous.pid, &stopAgent{})
	}
	agent := newGsAgent(uid, generation, open.stream, ctx.Self(), state.pid, state.activityPID)
	agentPID := ctx.System().SpawnOne(agent)
	g.sessions[uid] = session{
		pid:         agentPID,
		playerPID:   state.pid,
		activityPID: state.activityPID,
		generation:  generation,
		uid:         uid,
	}
	if !ctx.Send(agentPID, &inboundFrame{msg: open.msg}) {
		delete(g.sessions, uid)
		ctx.Send(agentPID, &stopAgent{})
		ctx.Send(state.pid, &ipb.SessionClosed{UID: uid, Generation: generation})
		ctx.Send(state.activityPID, &ipb.SessionClosed{UID: uid, Generation: generation})
		return nil, fmt.Errorf("could not send initial login message to gateway agent")
	}
	tlog.Log.Info("[gate/GateActor.createSession] 玩家网关会话已建立, pid=%v generation=%v playerPID=%v",
		uid, generation, state.pid)
	return &openedSession{pid: agentPID, uid: uid, generation: generation}, nil
}

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
	g.notifySessionClosed(ctx, closed.uid, current.playerPID, current.activityPID, current.generation)
	tlog.Log.Info("[gate/GateActor.onCloseSession] 网关会话已关闭, pid=%v reason=%v", closed.uid, closed.reason)
}

func (g *GateActor) onPushToMsg(ctx tree.Context, msg interface{}) {
	push, ok := msg.(*ipb.PushToMsg)
	if !ok || push == nil || push.Msg == nil {
		return
	}
	current, ok := g.sessions[push.UID]
	if !ok {
		return
	}
	if !ctx.TrySend(current.pid, push) {
		g.closeCurrent(ctx, push.UID, current, "outbound mailbox is full")
	}
}

func (g *GateActor) onPlayerMessage(ctx tree.Context, msg interface{}) {
	if g.shuttingDown {
		tlog.Log.Error("[gate/GateActor.onPlayerMessage] 服务关停期间拒绝玩家系统消息, type=%T", msg)
		return
	}
	command, ok := msg.(*ipb.PlayerMessage)
	if !ok || command == nil || command.UID == 0 || command.Message == nil {
		tlog.Log.Error("[gate/GateActor.onPlayerMessage] 玩家系统消息无效, type=%T", msg)
		return
	}
	state := g.playerActors[command.UID]
	if isPlayerActivityMessage(command.Message) {
		activityMessage := &ipb.PlayerActivityMessage{UID: command.UID, Message: command.Message}
		if state == nil {
			activityPID := g.spawnPlayerActivityActor(ctx, command.UID, []interface{}{activityMessage})
			g.playerActors[command.UID] = &playerActorState{activityPID: activityPID}
			tlog.Log.Info("[gate/GateActor.onPlayerMessage] 为离线玩家创建活动 Actor, pid=%v activityPID=%v type=%T", command.UID, activityPID, command.Message)
			return
		}
		if state.activityPID.ID == 0 {
			state.activityPID = g.spawnPlayerActivityActor(ctx, command.UID, []interface{}{activityMessage})
			return
		}
		if !ctx.TrySend(state.activityPID, activityMessage) {
			tlog.Log.Error("[gate/GateActor.onPlayerMessage] 玩家活动 Actor 不可用或队列已满, pid=%v activityPID=%v type=%T",
				command.UID, state.activityPID, command.Message)
		}
		return
	}
	if state == nil {
		playerPID := g.spawnPlayerActor(ctx, command.UID, []interface{}{command.Message})
		g.playerActors[command.UID] = &playerActorState{pid: playerPID}
		tlog.Log.Info("[gate/GateActor.onPlayerMessage] 为离线玩家创建临时 Actor, pid=%v playerPID=%v type=%T", command.UID, playerPID, command.Message)
		return
	}
	if state.pid.ID == 0 {
		state.pid = g.spawnPlayerActor(ctx, command.UID, []interface{}{command.Message})
		tlog.Log.Info("[gate/GateActor.onPlayerMessage] 为离线玩家创建临时 PlayerActor, pid=%v playerPID=%v type=%T", command.UID, state.pid, command.Message)
		return
	}
	if state.evicting || !ctx.TrySend(state.pid, command.Message) {
		if !state.AddPending(command.Message) {
			tlog.Log.Error("[gate/GateActor.onPlayerMessage] 玩家系统消息缓存已满, pid=%v type=%T", command.UID, command.Message)
			return
		}
		tlog.Log.Info("[gate/GateActor.onPlayerMessage] 玩家 Actor 正忙，系统消息已排队, pid=%v type=%T", command.UID, command.Message)
		g.schedulePlayerMessageRetry()
	}
}

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
	if state.evicting {
		return
	}
	if state.hasPending() {
		g.flushPending(state)
		g.schedulePlayerMessageRetry()
		return
	}
	state.evicting = true
	if !ctx.Send(state.pid, &ipb.StopPlayerActor{UID: ready.UID}) {
		tlog.Log.Error("[gate/GateActor.onPlayerActorEvictReady] 请求停止离线玩家 Actor 失败，将由重试定时器重试, pid=%v playerPID=%v", ready.UID, ready.PID)
	}
	g.schedulePlayerMessageRetry()
}

func (g *GateActor) onPlayerActorStopped(ctx tree.Context, msg interface{}) {
	stopped, ok := msg.(*ipb.PlayerActorStopped)
	if !ok || stopped == nil {
		return
	}
	state := g.playerActors[stopped.UID]
	if state == nil || state.pid != stopped.PID {
		return
	}
	pendingMessages := state.TakePending()
	state.pid = tree.PID{}
	state.evicting = false
	if open, exists := g.pendingOpen[stopped.UID]; exists {
		delete(g.pendingOpen, stopped.UID)
		if g.shuttingDown {
			open.request.Respond(nil, fmt.Errorf("game gateway is shutting down"))
			if state.activityPID.ID == 0 {
				delete(g.playerActors, stopped.UID)
			}
			return
		}
		if openSessionCanceled(open) {
			open.request.Respond(nil, fmt.Errorf("gateway stream closed before player actor was ready"))
			if len(pendingMessages) > 0 {
				state.pid = g.spawnPlayerActor(ctx, stopped.UID, pendingMessages)
			}
			return
		}
		if _, banned := g.bannedUIDs[stopped.UID]; banned {
			open.request.Respond(nil, fmt.Errorf("uid %d is banned", stopped.UID))
			return
		}
		state.pid = g.spawnPlayerActor(ctx, stopped.UID, pendingMessages)
		opened, err := g.createSession(ctx, open)
		if err == nil && g.rejectCanceledOpen(ctx, open, opened) {
			opened = nil
			err = fmt.Errorf("gateway stream closed while opening session")
		}
		open.request.Respond(opened, err)
		return
	}
	if g.shuttingDown {
		if state.activityPID.ID == 0 {
			delete(g.playerActors, stopped.UID)
		}
		return
	}
	if len(pendingMessages) > 0 {
		state.pid = g.spawnPlayerActor(ctx, stopped.UID, pendingMessages)
		tlog.Log.Info("[gate/GateActor.onPlayerActorStopped] 已重建玩家 Actor 并恢复排队消息, pid=%v playerPID=%v count=%v", stopped.UID, state.pid, len(pendingMessages))
	}
	if state.pid.ID == 0 && state.activityPID.ID == 0 {
		delete(g.playerActors, stopped.UID)
	}
}

// beginServerShutdown 停止网关接入，并协调玩家角色完成关服流程。
func (g *GateActor) beginServerShutdown(ctx tree.Context, message *sspb.Event) {
	if g.shuttingDown {
		ctx.Response(nil, fmt.Errorf("server shutdown is already in progress"))
		return
	}
	serverClosingEvent := gameevent.NewServerClosing()
	if gameevent.IsServerClosing(message) {
		serverClosingEvent = message
	}
	g.shuttingDown = true
	g.shutdownWaiter = ctx.RequestEnvelope()
	g.shutdownRemaining = 0
	g.shutdownErr = nil
	if g.messageRetryWheel != nil {
		g.messageRetryWheel.Stop()
	}
	if g.messageRetryTimer != nil {
		g.messageRetryTimer.Stop()
		g.messageRetryTimer = nil
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
		g.notifySessionClosed(ctx, uid, current.playerPID, current.activityPID, current.generation)
	}
	for uid, state := range g.playerActors {
		if state.evicting && state.hasPending() {
			pending := state.TakePending()
			playerPID := g.spawnPlayerActor(ctx, uid, pending)
			state.pid = playerPID
			state.evicting = false
		}
		if state.pid.ID != 0 {
			if !state.evicting {
				g.flushPending(state)
			}
			ctx.Send(state.pid, gameevent.Clone(serverClosingEvent))
			g.shutdownRemaining++
			ctx.RequestCallback(state.pid, &ipb.StopPlayerActor{UID: uid, Shutdown: true}, func(callbackCtx tree.Context, _ interface{}, err error) {
				g.onPlayerShutdownComplete(callbackCtx, uid, err)
			})
		}
		if state.activityPID.ID != 0 {
			g.shutdownRemaining++
			ctx.RequestCallback(state.activityPID, gameevent.Clone(serverClosingEvent), func(callbackCtx tree.Context, _ interface{}, err error) {
				g.onPlayerActivityShutdownComplete(callbackCtx, uid, err)
			})
		}
	}
	if g.shutdownRemaining == 0 {
		g.finishServerShutdown()
	}
	tlog.Log.Info("[gate/GateActor.beginServerShutdown] 已停止网关接入并等待玩家角色完成关服, count=%v", g.shutdownRemaining)
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

// onPlayerActivityShutdownComplete 记录玩家活动角色的关服结果并更新等待数量。
func (g *GateActor) onPlayerActivityShutdownComplete(_ tree.Context, uid uint64, err error) {
	if err != nil && g.shutdownErr == nil {
		g.shutdownErr = err
		tlog.Log.Error("[gate/GateActor.onPlayerActivityShutdownComplete] 玩家活动角色关服处理失败, pid=%v err=%v", uid, err)
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

func (g *GateActor) onUnbanUID(_ tree.Context, msg interface{}) {
	unban, ok := msg.(*ipb.UnbanUID)
	if !ok || unban == nil {
		return
	}
	delete(g.bannedUIDs, unban.UID)
}

func (g *GateActor) closeCurrent(
	ctx tree.Context,
	uid uint64,
	current session,
	reason string,
) {
	delete(g.sessions, uid)
	ctx.Send(current.pid, &stopAgent{})
	g.notifySessionClosed(ctx, uid, current.playerPID, current.activityPID, current.generation)
	tlog.Log.Info("[gate/GateActor.closeCurrent] 网关会话已关闭, pid=%v reason=%v", uid, reason)
}

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

func (g *GateActor) notifySessionClosed(
	ctx tree.Context,
	uid uint64,
	playerPID tree.PID,
	activityPID tree.PID,
	generation uint64,
) {
	if playerPID.ID != 0 && !ctx.Send(playerPID, &ipb.SessionClosed{UID: uid, Generation: generation}) {
		tlog.Log.Error("[gate/GateActor.notifySessionClosed] 通知玩家 Actor 断线失败, pid=%v playerPID=%v", uid, playerPID)
	}
	if activityPID.ID != 0 && !ctx.Send(activityPID, &ipb.SessionClosed{UID: uid, Generation: generation}) {
		tlog.Log.Error("[gate/GateActor.notifySessionClosed] 通知玩家活动 Actor 断线失败, pid=%v activityPID=%v", uid, activityPID)
	}
}

func (g *GateActor) onPlayerMessageRetryTimer() {
	g.messageRetryTimer = nil
	if g.shuttingDown {
		return
	}
	for uid, state := range g.playerActors {
		if state == nil {
			continue
		}
		if state.evicting && state.pid.ID != 0 && g.system != nil {
			if !g.system.TrySend(state.pid, &ipb.StopPlayerActor{UID: uid}) {
				tlog.Log.Error("[gate/GateActor.onPlayerMessageRetryTimer] 重试停止离线玩家 Actor 失败, pid=%v playerPID=%v", uid, state.pid)
			}
			continue
		}
		if state.hasPending() {
			g.flushPending(state)
		}
	}
	g.schedulePlayerMessageRetry()
}

func (g *GateActor) flushPending(state *playerActorState) {
	if state == nil || state.evicting || state.pid.ID == 0 || g.system == nil {
		return
	}
	pending := state.TakePending()
	for index, message := range pending {
		if !g.system.TrySend(state.pid, message) {
			for _, remaining := range pending[index:] {
				if !state.AddPending(remaining) {
					tlog.Log.Error("[gate/GateActor.flushPending] 玩家系统消息缓存已满, playerPID=%v", state.pid)
					break
				}
			}
			return
		}
	}
}

func (g *GateActor) schedulePlayerMessageRetry() {
	if g.messageRetryWheel == nil || g.messageRetryTimer != nil || g.shuttingDown {
		return
	}
	if !g.hasPlayerMessageRetryWork() {
		return
	}
	g.messageRetryTimer = g.messageRetryWheel.After(playerMessageRetryTimerType, playerMessageRetryInterval, nil)
	if g.messageRetryTimer == nil {
		tlog.Log.Error("[gate/GateActor.schedulePlayerMessageRetry] 创建玩家系统消息重试定时器失败")
	}
}

func (g *GateActor) hasPlayerMessageRetryWork() bool {
	for _, state := range g.playerActors {
		if state == nil || state.pid.ID == 0 {
			continue
		}
		if state.evicting || state.hasPending() {
			return true
		}
	}
	return false
}

func (state *playerActorState) hasPending() bool {
	return state != nil && len(state.pending) > 0
}

func isPlayerActivityMessage(message interface{}) bool {
	switch message.(type) {
	case *pb_activity.GetActivityListReq:
		return true
	case *pb_activity.JoinActivityReq:
		return true
	case *pb_activity.GetProgressReq:
		return true
	case *pb_activity.ClaimRewardReq:
		return true
	default:
		return false
	}
}

func validateFrame(frame *pb_gateway.Frame) error {
	if frame == nil {
		return fmt.Errorf("empty gateway frame")
	}
	if len(frame.GetPayload()) == 0 {
		return fmt.Errorf("empty gateway payload")
	}
	return nil
}

func openSessionCanceled(open *openSession) bool {
	return open == nil || open.stream == nil || open.stream.Context().Err() != nil
}

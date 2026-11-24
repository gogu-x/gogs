package itest

import (
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"

	"github.com/gogu-x/gogs/def"
	playinternal "github.com/gogu-x/gogs/game/play/internal"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
)

// recorderActor 把收到的消息转发到通道，用于模拟会话代理或游戏网关。
type recorderActor struct {
	name     string
	mailbox  int
	block    chan struct{}
	received chan interface{}
}

func newRecorder(name string) *recorderActor {
	return &recorderActor{name: name, mailbox: 16, received: make(chan interface{}, 16)}
}

func (r *recorderActor) Name() string        { return r.name }
func (r *recorderActor) MailboxSize() int    { return r.mailbox }
func (r *recorderActor) OnInit(tree.Context) {}
func (r *recorderActor) OnStop(tree.Context) {}
func (r *recorderActor) HandleMessage(ctx tree.Context, m interface{}) {
	if r.block != nil {
		<-r.block
	}
	if forwarded, ok := m.(*ipb.ForwardedRequest); ok {
		req, _ := forwarded.GetRequest().(*pb_guild.GetGuildReq)
		r.received <- guildForwarded{
			request: req, generation: forwarded.GetGeneration(),
			hasEnvelope: ctx.RequestEnvelope() != nil,
		}
		return
	}
	if _, ok := m.(*mongorpc.FindOne); ok {
		r.received <- ctx.RequestEnvelope()
		return
	}
	r.received <- m
}

// spawnRecorder 启动记录 Actor，并在测试结束时停止。
func spawnRecorder(t *testing.T, recorder *recorderActor) tree.PID {
	t.Helper()
	pid := tree.SpawnOne(recorder)
	t.Cleanup(func() {
		if recorder.block != nil {
			close(recorder.block)
		}
		tree.Stop(pid)
	})
	return pid
}

// pushReq 驱动玩家 Actor 在自身协程内调用 CastPlayerMsg。
type pushReq struct{}

// spawnPushingPlayer 启动一个收到 pushReq 时向自己推送登录响应的玩家 Actor。
func spawnPushingPlayer(t *testing.T) tree.PID {
	t.Helper()
	return spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterSysMsg(actor, (*pushReq)(nil), func(ctx *core.Context, _ *pushReq) {
			ctx.Response(ctx.CastPlayerMsg(testUID, &pb_auth.LoginGameAck{Uid: testUID}), nil)
		})
	})
}

func requestPush(t *testing.T, playerPID tree.PID) bool {
	t.Helper()
	got, err := tree.Request(playerPID, &pushReq{}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("推送请求失败: %v", err)
	}
	return got == true
}

func receive[T any](t *testing.T, recorder *recorderActor) T {
	t.Helper()
	select {
	case message := <-recorder.received:
		typed, ok := message.(T)
		if !ok {
			t.Fatalf("收到 %T, want %T", message, *new(T))
		}
		return typed
	case <-time.After(time.Second):
		t.Fatalf("超时未收到 %T", *new(T))
	}
	panic("unreachable")
}

// TestPushFindsSessionAgentByName 验证推送按命名规则找到会话代理，并携带当前会话版本。
func TestPushFindsSessionAgentByName(t *testing.T) {
	agent := newRecorder(def.SessionAgentName(testUID))
	spawnRecorder(t, agent)
	playerPID := spawnPushingPlayer(t)

	tree.Send(playerPID, &ipb.PlayerSessionOpened{UID: testUID, Generation: 7})
	if !requestPush(t, playerPID) {
		t.Fatal("直接推送应成功")
	}
	push := receive[*ipb.PushToMsg](t, agent)
	if push.UID != testUID || push.Generation != 7 {
		t.Fatalf("push uid=%v generation=%v, want uid=%v generation=7", push.UID, push.Generation, testUID)
	}
	if _, ok := push.Msg.(*pb_auth.LoginGameAck); !ok {
		t.Fatalf("push msg = %T, want *pb_auth.LoginGameAck", push.Msg)
	}
}

// guildForwarded 记录联盟 Actor 收到的转发请求及其会话代次。
type guildForwarded struct {
	request     *pb_guild.GetGuildReq
	generation  uint64
	hasEnvelope bool
}

// TestGuildRequestCarriesSessionGeneration 验证联盟请求以 ForwardedRequest 转交，携带原始请求和会话代次。
func TestGuildRequestCarriesSessionGeneration(t *testing.T) {
	guild := newRecorder(def.Guild)
	spawnRecorder(t, guild)
	player := spawnPlayerActor(t, playinternal.InitRoutes)
	request := &pb_guild.GetGuildReq{UID: testUID, GuildId: 1}

	tree.Send(player, &ipb.PlayerSessionOpened{UID: testUID, Generation: 7})
	tree.Send(player, &ipb.PlayerInbound{UID: testUID, Generation: 7, Message: request})

	message := receive[guildForwarded](t, guild)
	if message.generation != 7 || message.request != request || message.hasEnvelope {
		t.Fatalf("转发的会话代次或请求不匹配，收到 %+v", message)
	}
}

// TestForwardTargetUnavailablePushesErrorNtf 验证转发目标 Actor 不存在时向客户端推送 ErrorNtf(INTERNAL)。
func TestForwardTargetUnavailablePushesErrorNtf(t *testing.T) {
	agent := newRecorder(def.SessionAgentName(testUID))
	spawnRecorder(t, agent)
	player := spawnPlayerActor(t, playinternal.InitRoutes)
	request := &pb_guild.GetGuildReq{UID: testUID, GuildId: 1}

	tree.Send(player, &ipb.PlayerSessionOpened{UID: testUID, Generation: 7})
	tree.Send(player, &ipb.PlayerInbound{UID: testUID, Generation: 7, Message: request})

	for {
		push := receive[*ipb.PushToMsg](t, agent)
		ntf, ok := push.Msg.(*pb_common.ErrorNtf)
		if !ok {
			continue // 登录等其他推送
		}
		if push.Generation != 7 || ntf.GetCode() != pb_common.ErrCode_INTERNAL || ntf.GetRequestType() != string(proto.MessageName(request)) {
			t.Fatalf("错误通知不符: generation=%v ntf=%+v", push.Generation, ntf)
		}
		return
	}
}

// TestGuildRequestWaitingForPlayerLoad 验证玩家加载时换线后只转发新会话的请求。
func TestGuildRequestWaitingForPlayerLoad(t *testing.T) {
	mongo := newRecorder(def.Mongo)
	spawnRecorder(t, mongo)
	guild := newRecorder(def.Guild)
	spawnRecorder(t, guild)
	pid := tree.SpawnOne(&core.PlayerActor{UID: testUID, Boot: playinternal.InitRoutes})
	t.Cleanup(func() { tree.Stop(pid) })
	load := receive[*tree.Envelope](t, mongo)

	tree.Send(pid, &ipb.PlayerSessionOpened{UID: testUID, Generation: 7})
	tree.Send(pid, &ipb.PlayerInbound{
		UID: testUID, Generation: 7, Message: &pb_guild.GetGuildReq{UID: testUID, GuildId: 1},
	})
	tree.Send(pid, &ipb.PlayerSessionOpened{UID: testUID, Generation: 8})
	request := &pb_guild.GetGuildReq{UID: testUID, GuildId: 2}
	for range 2 {
		tree.Send(pid, &ipb.PlayerInbound{UID: testUID, Generation: 8, Message: request})
	}
	load.Respond(&player.Player{UID: testUID}, nil)

	for range 2 {
		message := receive[guildForwarded](t, guild)
		if message.generation != 8 || message.request != request || message.hasEnvelope {
			t.Fatalf("加载后应只转发新会话的普通请求，收到 %+v", message)
		}
	}
}

// TestPushWithoutSessionAgentFails 验证玩家没有在线会话代理时推送失败且不请求关闭会话。
func TestPushWithoutSessionAgentFails(t *testing.T) {
	gate := newRecorder(def.GameGate)
	spawnRecorder(t, gate)
	playerPID := spawnPushingPlayer(t)

	if requestPush(t, playerPID) {
		t.Fatal("没有会话代理时推送应失败")
	}
	select {
	case message := <-gate.received:
		t.Fatalf("不应向网关发送消息: %T", message)
	case <-time.After(20 * time.Millisecond):
	}
}

// TestPushFullAgentRequestsSessionClose 验证会话代理队列已满时推送失败，并请求网关关闭该代理的会话。
func TestPushFullAgentRequestsSessionClose(t *testing.T) {
	gate := newRecorder(def.GameGate)
	spawnRecorder(t, gate)
	agent := newRecorder(def.SessionAgentName(testUID))
	agent.mailbox = 1
	agent.block = make(chan struct{})
	agentPID := spawnRecorder(t, agent)
	// 第一条被处理并阻塞，第二条占满容量为 1 的队列。
	tree.Send(agentPID, struct{}{})
	time.Sleep(10 * time.Millisecond)
	tree.Send(agentPID, struct{}{})
	playerPID := spawnPushingPlayer(t)

	tree.Send(playerPID, &ipb.PlayerSessionOpened{UID: testUID, Generation: 9})
	if requestPush(t, playerPID) {
		t.Fatal("会话代理队列已满时推送应失败")
	}
	closeReq := receive[*ipb.CloseSession](t, gate)
	if closeReq.UID != testUID || closeReq.AgentPID != agentPID {
		t.Fatalf("close uid=%v agent=%v, want uid=%v agent=%v", closeReq.UID, closeReq.AgentPID, testUID, agentPID)
	}
}

// TestShutdownDrainsRoutedRequests 验证加载期间收到关服时，先执行已受理业务再回复关服。
func TestShutdownDrainsRoutedRequests(t *testing.T) {
	mongo := newRecorder(def.Mongo)
	spawnRecorder(t, mongo)
	pid := tree.SpawnOne(&core.PlayerActor{UID: testUID, Boot: func(actor *core.PlayerActor) {
		core.RegisterSysMsg(actor, (*testReq)(nil), func(ctx *core.Context, req *testReq) { ctx.Response(req.GetUID(), nil) })
	}})
	t.Cleanup(func() { tree.Stop(pid) })
	load := receive[*tree.Envelope](t, mongo)
	req := &testReq{UID: testUID}
	reply := tree.NewRequest(req, tree.PID{}, nil, nil)
	stop := tree.Request(pid, &ipb.StopPlayerActor{UID: testUID, Shutdown: true, QueuedDeliveries: []interface{}{ipb.WrapWithReply(req, reply)}})
	load.Respond(&player.Player{UID: testUID}, nil)
	if result, err := reply.AwaitTimeout(time.Second); err != nil || result != testUID {
		t.Fatalf("暂存请求结果为 %v, %v", result, err)
	}
	if _, err := stop.AwaitTimeout(time.Second); err != nil {
		t.Fatalf("关服失败 %v", err)
	}
}

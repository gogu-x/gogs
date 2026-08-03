package base

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/game/play/module/player"
	"github.com/gogu-x/gogs/natsrpc"
	_ "github.com/gogu-x/gogs/pb/pbregister"
	"github.com/gogu-x/gogs/pb/protoChat"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/gogs/pb/protoGuild"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
	"google.golang.org/protobuf/proto"
)

// stubCtx 是 tree.Context 的最小实现，仅供单测使用。
type stubCtx struct{}

func (stubCtx) Self() tree.PID                                 { return tree.PID{Name: "play"} }
func (stubCtx) Sender() tree.PID                               { return tree.PID{} }
func (stubCtx) Message() interface{}                           { return nil }
func (stubCtx) Send(tree.PID, interface{}) bool                { return true }
func (stubCtx) TrySend(tree.PID, interface{}) bool             { return true }
func (stubCtx) Request(tree.PID, interface{}) *tree.Envelope   { return nil }
func (stubCtx) RequestCallback(tree.PID, interface{}, func(tree.Context, interface{}, error)) *tree.Envelope {
	return nil
}
func (stubCtx) Response(interface{}, error)        {}
func (stubCtx) RequestEnvelope() *tree.Envelope    { return nil }
func (stubCtx) Stop()                              {}
func (stubCtx) Lookup(string) (tree.PID, bool)     { return tree.PID{}, false }
func (stubCtx) Register(string)                    {}
func (stubCtx) System() *tree.Tree                 { return nil }
func (stubCtx) SetValue(string, interface{})       {}
func (stubCtx) GetValue(string) interface{}        { return nil }

func (stubCtx) AfterFunc(time.Duration, func(tree.Context)) *timer.WheelTimer { return nil }
func (stubCtx) CronFunc(*timer.CronExpr, func(tree.Context)) *timer.WheelCron { return nil }

// fakeSender 记录回包去向。
type fakeSender struct {
	gateID    string
	castFrame *protoGateway.Frame
	replyReq  *protoGateway.Frame
	replyResp *protoGateway.Frame
}

func (f *fakeSender) CastGate(gateID string, fr *protoGateway.Frame) error {
	f.gateID, f.castFrame = gateID, fr
	return nil
}

func (f *fakeSender) ReplyTo(req, resp *protoGateway.Frame) error {
	f.replyReq, f.replyResp = req, resp
	return nil
}

func newTestApp() (*App, *fakeSender) {
	app := NewApp()
	s := &fakeSender{}
	app.Sender = s
	return app, s
}

func frameOf(t *testing.T, uid uint64, msg proto.Message) *protoGateway.Frame {
	t.Helper()
	body, err := codec.ProtoCodec.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &protoGateway.Frame{Uid: uid, ConnId: 7, GateId: "1", Payload: body}
}

func TestDispatcherRegistration(t *testing.T) {
	app, _ := newTestApp()
	d := app.Dispatcher()

	RegisterAnon(d, &protoGateway.LoginReq{}, func(*PlayContext, *protoGateway.LoginReq) {})
	RegisterPlayer(d, &protoChat.ChatReq{}, func(*PlayContext, *protoChat.ChatReq) {})
	RegisterSys(d, &protoGuild.JoinGuildReq{}, func(*SysContext, *protoGuild.JoinGuildReq) {})

	if got := d.PlayerRouteCount(); got != 2 {
		t.Fatalf("PlayerRouteCount = %d, want 2", got)
	}
	if got := d.SysRouteCount(); got != 1 {
		t.Fatalf("SysRouteCount = %d, want 1", got)
	}
	if anon, ok := d.IsAnonymous(&protoGateway.LoginReq{}); !ok || !anon {
		t.Fatalf("LoginReq: anon=%v registered=%v, want true/true", anon, ok)
	}
	if anon, ok := d.IsAnonymous(&protoChat.ChatReq{}); !ok || anon {
		t.Fatalf("ChatReq: anon=%v registered=%v, want false/true", anon, ok)
	}
	// 系统路由不出现在玩家路由表里
	if _, ok := d.IsAnonymous(&protoGuild.JoinGuildReq{}); ok {
		t.Fatal("JoinGuildReq should not be a player route")
	}
}

func TestDispatcherDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration should panic")
		}
	}()
	app, _ := newTestApp()
	RegisterPlayer(app.Dispatcher(), &protoChat.ChatReq{}, func(*PlayContext, *protoChat.ChatReq) {})
	RegisterPlayer(app.Dispatcher(), &protoChat.ChatReq{}, func(*PlayContext, *protoChat.ChatReq) {})
}

func TestHandleFrameAnonymousRouteAllowsNilPlayer(t *testing.T) {
	app, _ := newTestApp()
	var called bool
	RegisterAnon(app.Dispatcher(), &protoGateway.LoginReq{}, func(s *PlayContext, _ *protoGateway.LoginReq) {
		called = true
		if s.Player != nil {
			t.Error("Player should be nil before login")
		}
		if s.UID != 100 || s.ConnID != 7 || s.GateId != "1" {
			t.Errorf("ctx meta = uid:%d conn:%d gate:%s", s.UID, s.ConnID, s.GateId)
		}
		if s.App != app {
			t.Error("ctx should carry the play module")
		}
		if s.Tree() == nil {
			t.Error("ctx should carry tree.Context")
		}
	})

	if ok := app.HandleFrame(stubCtx{}, frameOf(t, 100, &protoGateway.LoginReq{})); !ok {
		t.Fatal("HandleFrame = false, want true")
	}
	if !called {
		t.Fatal("handler not called")
	}
}

func TestHandleFrameRequiresOnlinePlayer(t *testing.T) {
	app, _ := newTestApp()
	var called int
	RegisterPlayer(app.Dispatcher(), &protoChat.ChatReq{}, func(s *PlayContext, req *protoChat.ChatReq) {
		called++
		if s.Player == nil || s.Player.UID != 100 {
			t.Errorf("Player = %v, want uid 100", s.Player)
		}
		if req.GetContent() != "hi" {
			t.Errorf("content = %q", req.GetContent())
		}
	})

	f := frameOf(t, 100, &protoChat.ChatReq{Content: "hi"})

	// 未登录：拒绝
	if ok := app.HandleFrame(stubCtx{}, f); ok {
		t.Fatal("HandleFrame = true for offline player, want false")
	}
	if called != 0 {
		t.Fatal("handler must not run for offline player")
	}

	// 登录后：通过
	app.Players.Add(player.NewPlayerData(100))
	if ok := app.HandleFrame(stubCtx{}, f); !ok {
		t.Fatal("HandleFrame = false for online player, want true")
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

func TestHandleFrameUnregisteredType(t *testing.T) {
	app, _ := newTestApp()
	app.Players.Add(player.NewPlayerData(100))
	if ok := app.HandleFrame(stubCtx{}, frameOf(t, 100, &protoGuild.JoinGuildReq{})); ok {
		t.Fatal("HandleFrame = true for unregistered type, want false")
	}
}

func TestHandleFrameDisconnectRemovesPlayer(t *testing.T) {
	app, _ := newTestApp()
	app.Players.Add(player.NewPlayerData(100))
	f := &protoGateway.Frame{Uid: 100, MsgType: natsrpc.MsgTypeDisconnect}
	if ok := app.HandleFrame(stubCtx{}, f); !ok {
		t.Fatal("disconnect frame should be handled")
	}
	if app.Players.Count() != 0 {
		t.Fatalf("online count = %d, want 0", app.Players.Count())
	}
}

func TestHandleSystemSkipsLoginCheck(t *testing.T) {
	app, _ := newTestApp()
	var got uint64
	// 其他模块投递的消息在参数里带 uid，由 handler 自己决定是否查玩家
	RegisterSys(app.Dispatcher(), &protoGuild.JoinGuildReq{}, func(s *SysContext, req *protoGuild.JoinGuildReq) {
		got = req.GetUid()
		if s.Players() != app.Players {
			t.Error("SysContext should expose the player manager")
		}
	})

	if ok := app.HandleSystem(stubCtx{}, &protoGuild.JoinGuildReq{Uid: 42}); !ok {
		t.Fatal("HandleSystem = false, want true")
	}
	if got != 42 {
		t.Fatalf("uid = %d, want 42", got)
	}
	if ok := app.HandleSystem(stubCtx{}, &protoChat.ChatReq{}); ok {
		t.Fatal("HandleSystem = true for unregistered type, want false")
	}
}

func TestReplyRouting(t *testing.T) {
	app, sender := newTestApp()

	// 无 RequestId：Cast 到 gate.out.{gateID}
	s := &PlayContext{SysContext: SysContext{App: app, ctx: stubCtx{}}, UID: 100, ConnID: 7, GateId: "3"}
	s.Reply(&protoChat.ChatAck{State: 2})
	if sender.gateID != "3" || sender.castFrame == nil {
		t.Fatalf("cast gateID = %q, frame = %v", sender.gateID, sender.castFrame)
	}
	if sender.castFrame.GetUid() != 100 || sender.castFrame.GetConnId() != 7 {
		t.Fatalf("cast frame meta = %v", sender.castFrame)
	}
	decoded, err := codec.ProtoCodec.Unmarshal(sender.castFrame.GetPayload())
	if err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if ack, ok := decoded.(*protoChat.ChatAck); !ok || ack.GetState() != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}

	// 有 RequestId：回到发起方 inbox
	req := &protoGateway.Frame{Uid: 100, RequestId: "_INBOX.abc.1"}
	s2 := &PlayContext{SysContext: SysContext{App: app, ctx: stubCtx{}}, UID: 100, RequestId: req.RequestId, frame: req}
	s2.Reply(&protoChat.ChatAck{State: 1})
	if sender.replyResp == nil || sender.replyReq != req {
		t.Fatalf("replyReq = %v, replyResp = %v", sender.replyReq, sender.replyResp)
	}
	if sender.replyResp.GetRequestId() != "_INBOX.abc.1" {
		t.Fatalf("reply requestId = %q", sender.replyResp.GetRequestId())
	}
}

package internal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

func TestMain(m *testing.M) {
	logger, err := tlog.New("error", "", tlog.NoFlags)
	if err != nil {
		panic(err)
	}
	tlog.Log = logger
	os.Exit(m.Run())
}

type fakePublisher struct {
	subject string
	data    []byte
	err     error
}

func (p *fakePublisher) Publish(subject string, data []byte) error {
	p.subject, p.data = subject, append([]byte(nil), data...)
	return p.err
}

type fakeCodec struct {
	payloadErr error
	wrapperErr error
	decode     interface{}
	decodeErr  error
	ntf        *sspb.NatsMsgNtf
	req        *sspb.NatsMsgReq
}

func (c *fakeCodec) Marshal(msg interface{}) ([]byte, error) {
	switch m := msg.(type) {
	case *sspb.NatsMsgNtf:
		if c.wrapperErr != nil {
			return nil, c.wrapperErr
		}
		c.ntf = m
		return []byte("ntf"), nil
	case *sspb.NatsMsgReq:
		if c.wrapperErr != nil {
			return nil, c.wrapperErr
		}
		c.req = m
		return []byte("req"), nil
	case *sspb.NatsMsgAck:
		return []byte("ack"), nil
	default:
		if c.payloadErr != nil {
			return nil, c.payloadErr
		}
		return []byte("payload"), nil
	}
}

func (c *fakeCodec) Unmarshal([]byte) (interface{}, error) { return c.decode, c.decodeErr }

type routeContext struct {
	lookupName string
	target     tree.PID
	sentPID    tree.PID
	sent       interface{}
	request    *tree.Envelope
}

func (c *routeContext) Self() tree.PID       { return tree.PID{} }
func (c *routeContext) Sender() tree.PID     { return tree.PID{} }
func (c *routeContext) Message() interface{} { return nil }
func (c *routeContext) Send(pid tree.PID, msg interface{}) bool {
	c.sentPID, c.sent = pid, msg
	return true
}
func (c *routeContext) TrySend(pid tree.PID, msg interface{}) bool   { return c.Send(pid, msg) }
func (c *routeContext) Request(tree.PID, interface{}) *tree.Envelope { return nil }
func (c *routeContext) RequestCallback(tree.PID, interface{}, func(tree.Context, interface{}, error)) *tree.Envelope {
	return nil
}
func (c *routeContext) RequestAsMessage(tree.PID, interface{}) *tree.Envelope { return nil }
func (c *routeContext) Response(value interface{}, err error) {
	if c.request != nil {
		c.request.Respond(value, err)
	}
}
func (c *routeContext) RequestEnvelope() *tree.Envelope { return c.request }
func (c *routeContext) Stop()                           {}
func (c *routeContext) Lookup(name string) (tree.PID, bool) {
	c.lookupName = name
	return c.target, true
}
func (c *routeContext) Register(string)              {}
func (c *routeContext) System() *tree.Tree           { return nil }
func (c *routeContext) SetValue(string, interface{}) {}
func (c *routeContext) GetValue(string) interface{}  { return nil }

type testStopper struct{ stopped bool }

func (s *testStopper) Stop() { s.stopped = true }

type realTimerStopper struct{ timer *time.Timer }

func (s *realTimerStopper) Stop() { s.timer.Stop() }

func TestCastWrapsNotificationAndUsesModuleSubject(t *testing.T) {
	ns := NewActor("Gate", 7, 8, "")
	fc, pub := &fakeCodec{}, &fakePublisher{}
	ns.codec, ns.transport = fc, pub

	err := ns.catsMsg(&NatsMsg{TaggerModule: "Game", TaggerName: "player-42", ID: 1001, NodeId: 3, Msg: "hello", Cast: true})
	if err != nil {
		t.Fatalf("catsMsg: %v", err)
	}
	if pub.subject != Subject("Game", 1001, 3) {
		t.Fatalf("subject = %q", pub.subject)
	}
	if fc.ntf == nil || fc.ntf.TaggerName != "player-42" || string(fc.ntf.Msg) != "payload" {
		t.Fatalf("unexpected notification: %#v", fc.ntf)
	}
	if fc.ntf.SendModule != "Gate" || fc.ntf.Id != 1001 || fc.ntf.NodeID != 3 {
		t.Fatalf("notification routing metadata = %#v", fc.ntf)
	}
}

func TestNotificationRoutesToTargetActor(t *testing.T) {
	ns := NewActor("Game", 1, 1, "")
	ns.codec = &fakeCodec{decode: "decoded"}
	ctx := &routeContext{target: tree.PID{ID: 9, Name: "target"}}

	ns.NatsMsgNtf(ctx, &sspb.NatsMsgNtf{TaggerName: "target", Msg: []byte("wire")})
	if ctx.lookupName != "target" || ctx.sentPID != ctx.target || ctx.sent != "decoded" {
		t.Fatalf("route lookup=%q pid=%v msg=%v", ctx.lookupName, ctx.sentPID, ctx.sent)
	}
}

func TestCastPublishFailureRepliesToCaller(t *testing.T) {
	ns := NewActor("Gate", 7, 8, "")
	ns.codec = &fakeCodec{}
	ns.transport = &fakePublisher{err: errors.New("offline")}
	env := tree.NewRequest("cast", tree.PID{}, nil, nil)
	ctx := &routeContext{request: env}

	ns.HandleMessage(ctx, &NatsMsg{TaggerModule: "Game", TaggerName: "room", ID: 1, NodeId: 2, Msg: "notice", Cast: true})
	_, err := env.AwaitTimeout(time.Second)
	if err == nil || !strings.Contains(err.Error(), "publish cast") {
		t.Fatalf("cast response error = %v", err)
	}
}

func TestCallAckDeletesPendingAndStopsTimeout(t *testing.T) {
	ns := NewActor("Gate", 7, 8, "")
	fc, pub := &fakeCodec{decode: "reply"}, &fakePublisher{}
	ns.codec, ns.transport = fc, pub
	var timeoutID int64
	stopper := &testStopper{}
	ns.afterTimeout = func(_ time.Duration, id int64) stoppable { timeoutID = id; return stopper }
	env := tree.NewRequest("call", tree.PID{}, nil, nil)

	ns.sendCall(&NatsMsg{TaggerModule: "Game", TaggerName: "room", ID: 1001, NodeId: 3, Msg: "request", Timeout: time.Second}, env)
	if fc.req == nil || fc.req.Id != 7 || fc.req.NodeID != 8 {
		t.Fatalf("request origin route = %#v", fc.req)
	}
	if pub.subject != Subject("Game", 1001, 3) {
		t.Fatalf("request subject = %q", pub.subject)
	}
	if _, ok := ns.ReqMsgMap[timeoutID]; !ok {
		t.Fatalf("session %d is not pending", timeoutID)
	}

	ns.NatsMsgAck(nil, &sspb.NatsMsgAck{SessionID: timeoutID, Msg: []byte("ack")})
	value, err := env.AwaitTimeout(time.Second)
	if err != nil || value != "reply" {
		t.Fatalf("reply value=%v err=%v", value, err)
	}
	if len(ns.ReqMsgMap) != 0 || len(ns.requestTimers) != 0 || !stopper.stopped {
		t.Fatalf("pending state not cleared: requests=%d timers=%d stopped=%v", len(ns.ReqMsgMap), len(ns.requestTimers), stopper.stopped)
	}
}

func TestCallTimeoutRespondsAndDeletesPending(t *testing.T) {
	ns := NewActor("Gate", 7, 8, "")
	ns.codec, ns.transport = &fakeCodec{}, &fakePublisher{}
	ns.afterTimeout = func(d time.Duration, id int64) stoppable {
		h := &realTimerStopper{}
		h.timer = time.AfterFunc(d, func() { ns.timeoutRequest(id) })
		return h
	}
	env := tree.NewRequest("call", tree.PID{}, nil, nil)

	started := time.Now()
	ns.sendCall(&NatsMsg{TaggerModule: "Game", ID: 1, NodeId: 2, Msg: "request", Timeout: 20 * time.Millisecond}, env)
	_, err := env.AwaitTimeout(time.Second)
	if !errors.Is(err, tree.ErrTimeout) {
		t.Fatalf("timeout error = %v", err)
	}
	if time.Since(started) < 15*time.Millisecond {
		t.Fatalf("request timed out before configured duration")
	}
	if len(ns.ReqMsgMap) != 0 || len(ns.requestTimers) != 0 {
		t.Fatalf("timed out request retained")
	}
}

func TestCallFailuresRespondWithoutPending(t *testing.T) {
	tests := []struct {
		name  string
		codec *fakeCodec
		pub   *fakePublisher
		want  string
	}{
		{name: "payload encoding", codec: &fakeCodec{payloadErr: errors.New("encode")}, pub: &fakePublisher{}, want: "marshal call payload"},
		{name: "wrapper encoding", codec: &fakeCodec{wrapperErr: errors.New("encode")}, pub: &fakePublisher{}, want: "marshal request"},
		{name: "publish", codec: &fakeCodec{}, pub: &fakePublisher{err: errors.New("offline")}, want: "publish call"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := NewActor("Gate", 7, 8, "")
			ns.codec, ns.transport = tt.codec, tt.pub
			env := tree.NewRequest("call", tree.PID{}, nil, nil)
			ns.sendCall(&NatsMsg{TaggerModule: "Game", ID: 1, NodeId: 2, Msg: "request"}, env)
			_, err := env.AwaitTimeout(time.Second)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
			if len(ns.ReqMsgMap) != 0 {
				t.Fatalf("failed request retained: %d", len(ns.ReqMsgMap))
			}
		})
	}
}

func TestOnStopIsNilSafe(t *testing.T) {
	ns := NewActor("Gate", 1, 1, "")
	ns.OnStop(nil)
}

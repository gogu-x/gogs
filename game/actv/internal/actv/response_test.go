package actv

import (
	"errors"
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// directContext 验证 Actv 按名称查找会话代理并直接投递，无 Player 回调信封。
type directContext struct {
	tree.Context
	reply  *ipb.PushToMsg
	target tree.PID
	sent   int
}

func (c *directContext) RequestEnvelope() *tree.Envelope { return nil }
func (c *directContext) Lookup(name string) (tree.PID, bool) {
	if name == def.SessionAgentName(7) {
		return testAgentPID, true
	}
	return tree.PID{ID: 1}, name == def.GameGate
}
func (c *directContext) Send(pid tree.PID, value interface{}) bool {
	c.reply = value.(*ipb.PushToMsg)
	c.target = pid
	c.sent++
	return true
}
func (c *directContext) TrySend(pid tree.PID, value interface{}) bool { return c.Send(pid, value) }

func directRequest() *ipb.ForwardedRequest {
	return &ipb.ForwardedRequest{UID: 7, Generation: 3, Request: &pb_actv.GetActvListReq{}}
}

// testAgentPID 是测试玩家 7 当前会话代理的进程标识。
var testAgentPID = tree.PID{ID: 99, Name: "gateway-uid-7"}

func assertDirect(t *testing.T, c *directContext, code pb_common.ErrCode) {
	t.Helper()
	if c.sent != 1 || c.reply == nil || c.reply.GetUID() != 7 || c.reply.GetGeneration() != 3 || c.target != testAgentPID {
		t.Fatal("直发次数、玩家、会话版本或会话代理不符")
	}
	ack, ok := c.reply.Msg.(*pb_actv.GetActvListAck)
	if !ok || ack.GetCode() != code {
		t.Fatal("直发响应类型或错误码不符")
	}
}

// TestDirectActvLoaded 验证已加载玩家列表直接发往 Gate。
func TestDirectActvLoaded(t *testing.T) {
	a := &Actv{ready: true, actvService: service.NewActvService(1), players: make(map[uint64]*playerLoadState)}
	a.actvService.SetPlayerLoaded(7)
	c := &directContext{}
	a.registerHandlers()
	a.HandleMessage(c, directRequest())
	assertDirect(t, c, pb_common.ErrCode_OK)
}

// TestDirectActvPending 验证全局及玩家加载中的请求在恢复后仍直发 Gate。
func TestDirectActvPending(t *testing.T) {
	a := &Actv{actvService: service.NewActvService(1), players: make(map[uint64]*playerLoadState)}
	c := &directContext{}
	a.onForwardedRequest(c, directRequest())
	if len(a.pending) != 1 || c.sent != 0 {
		t.Fatal("全局加载期间未暂存")
	}
	a.ready = true
	a.actvService.SetPlayerLoaded(7)
	a.drainPending(c)
	assertDirect(t, c, pb_common.ErrCode_OK)
	c = &directContext{}
	a.actvService = service.NewActvService(1)
	state := &playerLoadState{loading: true}
	a.players[7] = state
	a.onForwardedRequest(c, directRequest())
	if len(state.pending) != 1 || c.sent != 0 {
		t.Fatal("玩家加载期间未暂存")
	}
	a.onPlayerLoadFinished(c, 7, state, nil)
	assertDirect(t, c, pb_common.ErrCode_OK)
}

// assertErrorNtf 验证活动请求无法处理时向原会话推送一次 ErrorNtf(INTERNAL)。
func assertErrorNtf(t *testing.T, c *directContext) {
	t.Helper()
	if c.sent != 1 || c.reply == nil || c.reply.GetGeneration() != 3 || c.target != testAgentPID {
		t.Fatalf("错误通知次数、会话版本或目标不符: sent=%v reply=%+v", c.sent, c.reply)
	}
	ntf, ok := c.reply.Msg.(*pb_common.ErrorNtf)
	if !ok || ntf.GetCode() != pb_common.ErrCode_INTERNAL || ntf.GetRequestType() != string(proto.MessageName(&pb_actv.GetActvListReq{})) {
		t.Fatalf("错误通知内容不符: %+v", c.reply.Msg)
	}
}

// TestDirectPendingFailure 验证玩家加载失败、暂存队列已满时推送 ErrorNtf。
func TestDirectPendingFailure(t *testing.T) {
	a := &Actv{ready: true, actvService: service.NewActvService(1), players: make(map[uint64]*playerLoadState)}
	state := &playerLoadState{loading: true}
	a.players[7] = state
	c := &directContext{}
	a.onForwardedRequest(c, directRequest())
	a.onPlayerLoadFinished(c, 7, state, errors.New("load failed"))
	assertErrorNtf(t, c)

	state = &playerLoadState{loading: true, pending: make([]pendingActvRequest, maxPendingPlayerRequests)}
	a.players[7] = state
	c = &directContext{}
	a.onForwardedRequest(c, directRequest())
	assertErrorNtf(t, c)

	a = &Actv{pending: make([]pendingActvRequest, maxPendingActvRequests)}
	c = &directContext{}
	a.onForwardedRequest(c, directRequest())
	assertErrorNtf(t, c)
}

// pushBehavior 由业务自主决定是否推送以及推送数量。
type pushBehavior struct {
	base.BaseBehavior
	count int
}

func (b *pushBehavior) HandleRequest(uid uint64, request proto.Message) {
	for i := 0; i < b.count; i++ {
		b.PushToPlayer(uid, request)
	}
}

// TestBusinessControlsPush 验证业务没有主动推送时框架不自动回包。
func TestBusinessControlsPush(t *testing.T) {
	svc := service.NewActvService(1)
	behavior := &pushBehavior{}
	if !svc.AddActv(&service.Actv{Meta: &service.ActvMeta{ActvID: 12, OwnerID: 7, OwnerScope: service.ScopePlayer}, Ext: behavior}) {
		t.Fatal("初始化失败")
	}
	svc.SetPlayerLoaded(7)
	actor := &Actv{ready: true, actvService: svc}
	ctx := &directContext{}
	actor.onForwardedRequest(ctx, &ipb.ForwardedRequest{UID: 7, Generation: 3, Request: &pb_actv.GetProgressReq{ActvId: 12}})
	if ctx.sent != 0 {
		t.Fatal("业务未推送却出现自动响应")
	}
}

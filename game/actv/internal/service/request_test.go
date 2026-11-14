package service_test

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// permissiveBehavior 模拟允许读取非本人或已结束活动的业务处理器。
type permissiveBehavior struct {
	base.BaseBehavior
	called bool
	code   pb_common.ErrCode
}

func (b *permissiveBehavior) HandleRequest(uid uint64, _ proto.Message) {
	b.called = true
	b.PushToPlayer(uid, &pb_actv.GetProgressAck{Code: b.code})
}

// TestRequestPoliciesBelongToBusiness 验证公共路由不覆盖业务对归属、开始和结束状态的决策。
func TestRequestPoliciesBelongToBusiness(t *testing.T) {
	tlog.NewLog("", 0)
	svc := service.NewActvService(1)
	behavior := &permissiveBehavior{}
	actv := &service.Actv{Meta: &service.ActvMeta{ActvID: 12, OwnerID: 99, OwnerScope: service.ScopePlayer, State: service.StateEnded, StartTs: time.Now().Unix() + 3600}, Ext: behavior}
	if !svc.AddActv(actv) {
		t.Fatal("活动初始化失败")
	}
	gate := &captureGate{messages: make(chan *ipb.PushToMsg, 8)}
	pid := tree.SpawnOne(gate)
	t.Cleanup(func() { tree.Stop(pid) })
	svc.HandleRequest(7, 12, &pb_actv.GetProgressReq{})
	response := receivePush(t, gate, 7).Msg
	if !behavior.called || response == nil {
		t.Fatal("公共路由提前施加业务限制")
	}
	behavior.code = pb_common.ErrCode_ACTV_NOT_OPEN
	svc.HandleRequest(7, 12, &pb_actv.GetProgressReq{})
	response = receivePush(t, gate, 7).Msg
	if notification, ok := response.(*pb_actv.GetProgressAck); !ok || notification.GetCode() != behavior.code {
		t.Fatal("业务错误码未透传")
	}
	// 请求已结束后仍能主动推送，并且显式 UID 不会串到上一位玩家。
	if !behavior.PushToPlayer(8, &pb_actv.GetProgressAck{}) {
		t.Fatal("无请求时推送失败")
	}
	receivePush(t, gate, 8)
	done := make(chan bool, 1)
	go func() { done <- behavior.PushToPlayer(9, &pb_actv.GetProgressAck{}) }()
	if !<-done {
		t.Fatal("异步主动推送失败")
	}
	receivePush(t, gate, 9)
	if behavior.PushToPlayer(0, &pb_actv.GetProgressAck{}) || behavior.PushToPlayer(7, (*pb_actv.GetProgressAck)(nil)) {
		t.Fatal("无效推送应拒绝")
	}
}

// captureGate 捕获通过真实 Tree 投递的按 UID 推送。
type captureGate struct{ messages chan *ipb.PushToMsg }

func (*captureGate) Name() string        { return def.GameGate }
func (*captureGate) OnInit(tree.Context) {}
func (*captureGate) OnStop(tree.Context) {}
func (g *captureGate) HandleMessage(_ tree.Context, message interface{}) {
	g.messages <- message.(*ipb.PushToMsg)
}

func receivePush(t *testing.T, gate *captureGate, uid uint64) *ipb.PushToMsg {
	t.Helper()
	select {
	case message := <-gate.messages:
		if message.GetUID() != uid || message.GetGeneration() != 0 {
			t.Fatal("推送没有按显式 UID 定位当前会话")
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("Gate 未收到推送")
		return nil
	}
}

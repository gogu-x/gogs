package ctl_actv

import (
	"testing"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// requestContext 模拟单向活动投递和 Gate 响应，不启动业务服务。
type requestContext struct {
	full bool
	tree.Context
	missing   bool
	forwarded *ipb.PlayerActvMessage
	reply     *ipb.PushToMsg
}

func (c *requestContext) Lookup(name string) (tree.PID, bool) {
	return tree.PID{ID: 1}, name == def.GameGate || (name == def.ActorActv && !c.missing)
}
func (c *requestContext) TrySend(_ tree.PID, message interface{}) bool {
	c.forwarded = message.(*ipb.PlayerActvMessage)
	return !c.full
}
func (c *requestContext) Send(_ tree.PID, message interface{}) bool {
	c.reply = message.(*ipb.PushToMsg)
	return true
}

// TestActvResponses 验证四个接口单向投递，不注册 Player 响应回调。
func TestActvResponses(t *testing.T) {
	tlog.NewLog("", 0)
	cases := []struct {
		name   string
		call   func(*core.Context)
		actvID uint64
	}{
		{"list", func(c *core.Context) { OnGetActvList(c, &pb_actv.GetActvListReq{}) }, 0},
		{"progress", func(c *core.Context) { OnGetActvProgress(c, &pb_actv.GetProgressReq{ActvId: 12}) }, 12},
		{"reward", func(c *core.Context) { OnClaimActvReward(c, &pb_actv.ClaimRewardReq{ActvId: 12}) }, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &requestContext{}
			ctx := &core.Context{Actor: &core.PlayerActor{Player: &player.Player{UID: 42}}, TreeCtx: fake}
			tc.call(ctx)
			if fake.forwarded.GetUID() != 42 || fake.forwarded.GetActvID() != tc.actvID {
				t.Fatal("转发身份或活动编号不符")
			}
			if fake.reply != nil {
				t.Fatal("成功投递后不应由 Player 回复")
			}
		})
	}
}

// TestUnavailableDoesNotInventResponse 验证投递失败不会由框架伪造固定 ACK 或错误通知。
func TestUnavailableDoesNotInventResponse(t *testing.T) {
	tlog.NewLog("", 0)
	for _, fake := range []*requestContext{{missing: true}, {full: true}} {
		ctx := &core.Context{Actor: &core.PlayerActor{Player: &player.Player{UID: 42}}, TreeCtx: fake}
		OnGetActvList(ctx, &pb_actv.GetActvListReq{})
		if fake.reply != nil {
			t.Fatal("框架不应自动发送固定失败消息")
		}
	}
	OnGetActvList(nil, &pb_actv.GetActvListReq{})
	OnGetActvList(nil, nil)
}

package itest

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
)

// panicReq 驱动一个会 panic 的玩家业务处理函数。
type panicReq struct{}

// TestDeliveryWithReplyAnswersOnHandlerPanic 验证业务处理函数 panic 时原调用方收到错误，
// 且玩家 Actor 继续处理后续投递。
func TestDeliveryWithReplyAnswersOnHandlerPanic(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterSysMsg(actor, (*panicReq)(nil), func(*core.Context, *panicReq) {
			panic("业务处理异常")
		})
		core.RegisterSysMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Response(ctx.PlayerID(), nil)
		})
	})

	failed := tree.NewRequest(nil, tree.PID{}, nil, nil)
	tree.Send(pid, ipb.WrapWithReply(&panicReq{}, failed))
	if _, err := failed.AwaitTimeout(time.Second); err == nil || err == tree.ErrTimeout {
		t.Fatalf("处理函数 panic 后调用方应收到错误, got %v", err)
	}

	succeeded := tree.NewRequest(nil, tree.PID{}, nil, nil)
	tree.Send(pid, ipb.WrapWithReply(&testReq{UID: testUID}, succeeded))
	if value, err := succeeded.AwaitTimeout(time.Second); err != nil || value != testUID {
		t.Fatalf("后续投递结果 = %v, %v; want %v", value, err, testUID)
	}
}

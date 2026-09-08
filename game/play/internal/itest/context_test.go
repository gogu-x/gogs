package itest

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
)

const (
	testTimer = timer.TimerType(99)
	testEvent = comm.EventName("testEvent")
	innerEvt  = comm.EventName("innerEvent")
)

// TestEventFromRequestKeepsRequestContext 验证请求中通过 Context.Emit 触发的
// 事件，监听者拿到的就是那条请求的 Context：Player 已注入且 Response 可用。
func TestEventFromRequestKeepsRequestContext(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		py.PlayerMgr.Add(player.NewPlayerData(testUID))
		py.OnEvent(testEvent, func(ctx *core.Context, _ *comm.Arg) {
			// 监听者直接用注入的 ctx 回复原请求
			ctx.Response(ctx.PlayerID(), nil)
		})
		core.RegisterPlayer(py, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Emit(testEvent, comm.NewArg())
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("监听者未拿到可回复的请求上下文: %v", err)
	}
	if got != testUID {
		t.Fatalf("ctx.UID() = %v, want %v", got, testUID)
	}
}

// TestEventOutsideRequestUsesSysCtx 验证非请求场景（开服）下监听者拿到 SysCtx：
// 能力齐全，但不携带 Player / Req。
func TestEventOutsideRequestUsesSysCtx(t *testing.T) {
	type result struct {
		playerNil bool
		reqNil    bool
		capsOK    bool
	}
	ch := make(chan result, 1)

	spawnPlay(t, func(py *core.Play) {
		py.OnEvent(core.ServerStart, func(ctx *core.Context, _ *comm.Arg) {
			ch <- result{
				playerNil: ctx.Player == nil,
				reqNil:    ctx.Req == nil,
				capsOK:    ctx.Players() != nil && ctx.Event() != nil && ctx.TimeWheel() != nil,
			}
		})
	})

	select {
	case got := <-ch:
		if !got.playerNil || !got.reqNil {
			t.Errorf("SysCtx 不应携带 Player/Req: %+v", got)
		}
		if !got.capsOK {
			t.Error("SysCtx 的公共能力不完整")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServerStart 事件未触发")
	}
}

// TestTimerHandlerGetsSysCtx 验证定时器回调拿到 SysCtx，且能力齐全。
func TestTimerHandlerGetsSysCtx(t *testing.T) {
	type result struct {
		playerNil bool
		online    int
	}
	ch := make(chan result, 1)

	spawnPlay(t, func(py *core.Play) {
		py.PlayerMgr.Add(player.NewPlayerData(testUID))
		py.OnTimer(testTimer, func(ctx *core.Context, _ interface{}) {
			ch <- result{playerNil: ctx.Player == nil, online: ctx.Players().Count()}
		})
		py.After(testTimer, 20*time.Millisecond, nil)
	})

	select {
	case got := <-ch:
		if !got.playerNil {
			t.Error("定时器 ctx 不应携带 Player")
		}
		if got.online != 1 {
			t.Errorf("ctx.Players().Count() = %d, want 1", got.online)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("定时器回调未触发")
	}
}

// TestNestedEmitRestoresContext 验证事件嵌套触发后上下文能正确恢复：
// 第一个监听者内部用 Play.Emit 触发了另一个事件（SysCtx），
// 返回后第二个监听者必须仍然拿到原请求的 Context。
func TestNestedEmitRestoresContext(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		py.PlayerMgr.Add(player.NewPlayerData(testUID))
		py.OnEvent(innerEvt, func(ctx *core.Context, _ *comm.Arg) {})
		// priority 越大越先执行
		py.OnEvent(testEvent, func(ctx *core.Context, _ *comm.Arg) {
			py.Emit(innerEvt, comm.NewArg()) // 期间上下文被切成 SysCtx
		}, 10)
		py.OnEvent(testEvent, func(ctx *core.Context, _ *comm.Arg) {
			ctx.Response(ctx.PlayerID(), nil)
		}, 0)
		core.RegisterPlayer(py, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Emit(testEvent, comm.NewArg())
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("嵌套 Emit 后上下文未恢复: %v", err)
	}
	if got != testUID {
		t.Fatalf("ctx.UID() = %v, want %v（上下文被内层 Emit 污染）", got, testUID)
	}
}

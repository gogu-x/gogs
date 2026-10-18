package itest

import (
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/tree/comm"
)

// testEvent 是事件监听测试使用的事件名称。
const testEvent = comm.EventName("testEvent")

// TestPlayerEventRunsOnOwningActor 验证玩家事件在该玩家角色内同步执行。
func TestPlayerEventRunsOnOwningActor(t *testing.T) {
	result := make(chan uint64, 1)
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		actor.OnEvent(testEvent, func(current *core.PlayerActor, _ *comm.Arg) {
			result <- current.GetPlayer().GetUID()
		})
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Emit(testEvent, comm.NewArg())
			ctx.Response(true, nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil || got != true {
		t.Fatalf("request response = %v, err = %v", got, err)
	}
	select {
	case uid := <-result:
		if uid != testUID {
			t.Fatalf("event ran for uid=%d, want %d", uid, testUID)
		}
	case <-time.After(time.Second):
		t.Fatal("player event was not invoked")
	}
}

// TestNestedEmitPreservesActorState 验证嵌套事件仍由同一个玩家角色串行处理。
func TestNestedEmitPreservesActorState(t *testing.T) {
	// innerEvent 是嵌套事件测试使用的事件名称。
	const innerEvent = comm.EventName("innerEvent")
	result := make(chan uint64, 1)
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		actor.OnEvent(innerEvent, func(current *core.PlayerActor, _ *comm.Arg) {
			if current.GetPlayer() != nil {
				current.GetPlayer().MarkDirty()
			}
		})
		actor.OnEvent(testEvent, func(current *core.PlayerActor, _ *comm.Arg) {
			current.Emit(innerEvent, comm.NewArg())
		})
		actor.OnEvent(testEvent, func(current *core.PlayerActor, _ *comm.Arg) {
			result <- current.GetPlayer().GetUID()
		})
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Emit(testEvent, comm.NewArg())
			ctx.Response(true, nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil || got != true {
		t.Fatalf("request response = %v, err = %v", got, err)
	}
	select {
	case uid := <-result:
		if uid != testUID {
			t.Fatalf("nested event changed actor uid to %d", uid)
		}
	case <-time.After(time.Second):
		t.Fatal("outer event listener was not invoked")
	}
}

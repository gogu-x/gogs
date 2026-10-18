package itest

import (
	"errors"
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/tree"
)

var errNoPlayer = errors.New("PlayerActor 数据未加载")

// testUID 是集成测试使用的玩家编号。
const testUID = uint64(42)

// testReq 满足 uidRequest，用于驱动 RegisterPlayerMsg。
type testReq struct{ UID uint64 }

func (r *testReq) GetUID() uint64 { return r.UID }

// spawnPlayerActor 启动一个只注册了初始化配置中路由的玩家角色。
func spawnPlayerActor(t *testing.T, boot func(*core.PlayerActor)) (*tree.Tree, tree.PID) {
	t.Helper()
	tr := tree.NewTree()
	actor := &core.PlayerActor{
		UID:    testUID,
		Player: player.NewPlayerData(testUID),
		Boot:   boot,
	}
	pid := tr.SpawnOne(actor)
	t.Cleanup(tr.Shutdown)
	return tr, pid
}

// TestContextResponseReachesRequester 验证 Context.TreeCtx 使用当前消息的上下文。
func TestContextResponseReachesRequester(t *testing.T) {
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Response(ctx.PlayerID(), nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("Response 未回到请求方: %v", err)
	}
	if got != testUID {
		t.Fatalf("uid = %v, want %v", got, testUID)
	}
}

// TestRegisterPlayerInjectsPlayer 验证请求上下文通过玩家角色访问玩家数据。
func TestRegisterPlayerInjectsPlayer(t *testing.T) {
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			if ctx.GetPlayer() == nil {
				ctx.Response(nil, errNoPlayer)
				return
			}
			ctx.Response(ctx.GetPlayer().GetUID(), nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != testUID {
		t.Fatalf("player UID = %v, want %v", got, testUID)
	}
}

// TestRegisterPlayerRejectsAnotherUID 验证请求不能访问其他玩家角色的数据。
func TestRegisterPlayerRejectsAnotherUID(t *testing.T) {
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			t.Error("handler 不应被调用：请求 UID 不属于当前 Actor")
		})
	})

	if _, err := tr.Request(pid, &testReq{UID: testUID + 1}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("UID 不匹配时应返回 error，实际为 nil")
	}
}

// TestRegisterSysUsesActorPlayer 验证系统消息也处理当前角色的玩家数据。
func TestRegisterSysUsesActorPlayer(t *testing.T) {
	tr, pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterSysMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Response(ctx.GetPlayer() != nil && ctx.PlayerID() == testUID, nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Fatal("RegisterSys 应注入当前 Actor 的玩家数据")
	}
}

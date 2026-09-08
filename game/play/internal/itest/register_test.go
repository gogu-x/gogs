package itest

import (
	"errors"
	"testing"
	"time"

	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/tree"
)

var errNoPlayer = errors.New("ctx.Player 未注入")

const testUID = uint64(42)

// testReq 满足 uidRequest，用于驱动 RegisterPlayer。
type testReq struct{ UID uint64 }

func (r *testReq) GetUID() uint64 { return r.UID }

// spawnPlay 启动一个只注册了 boot 里那些路由的 Play，返回其 PID。
func spawnPlay(t *testing.T, boot func(*core.Play)) (*tree.Tree, tree.PID) {
	t.Helper()
	tr := tree.NewTree()
	py := &core.Play{Boot: boot}
	pid := tr.SpawnOne(py)
	t.Cleanup(tr.Shutdown)
	return tr, pid
}

// TestContextResponseReachesRequester 验证 Context.TreeCtx 取的是本条消息的
// context：只有这样 Response 才能真正回复。若 TreeCtx 误用 OnInit 期捕获的
// context，其 request 字段恒为 nil，Response 静默失效，本用例会超时失败。
func TestContextResponseReachesRequester(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		py.PlayerMgr.Add(player.NewPlayerData(testUID))
		core.RegisterPlayer(py, (*testReq)(nil), func(ctx *core.Context, req *testReq) {
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

// TestRegisterPlayerInjectsPlayer 验证在线玩家由注册层预先注入 Context。
func TestRegisterPlayerInjectsPlayer(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		py.PlayerMgr.Add(player.NewPlayerData(testUID))
		core.RegisterPlayer(py, (*testReq)(nil), func(ctx *core.Context, req *testReq) {
			if ctx.Player == nil {
				ctx.Response(nil, errNoPlayer)
				return
			}
			ctx.Response(ctx.Player.UID, nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != testUID {
		t.Fatalf("ctx.Player.UID = %v, want %v", got, testUID)
	}
}

// TestRegisterPlayerOfflineRespondsError 验证玩家不在线时会显式回错，
// 而不是静默丢弃请求让调用方等到 DeadlineExceeded。
func TestRegisterPlayerOfflineRespondsError(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		core.RegisterPlayer(py, (*testReq)(nil), func(ctx *core.Context, req *testReq) {
			t.Error("handler 不应被调用：玩家不在线")
		})
	})

	if _, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("玩家不在线时应返回 error，实际为 nil")
	}
}

// TestRegisterSysHasNilPlayer 验证系统消息下 Context.Player 为 nil 且 UID 为 0。
func TestRegisterSysHasNilPlayer(t *testing.T) {
	tr, pid := spawnPlay(t, func(py *core.Play) {
		core.RegisterSys(py, (*testReq)(nil), func(ctx *core.Context, req *testReq) {
			ctx.Response(ctx.Player == nil && ctx.PlayerID() == 0, nil)
		})
	})

	got, err := tr.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Fatalf("RegisterSys 下 Player 应为 nil、UID 应为 0")
	}
}

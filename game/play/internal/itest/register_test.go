package itest

import (
	"errors"
	"testing"
	"time"

	playinternal "github.com/gogu-x/gogs/game/play/internal"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var errNoPlayer = errors.New("PlayerActor 数据未加载")

// testUID 是集成测试使用的玩家编号。
const testUID = uint64(42)

// testReq 满足 uidRequest，用于驱动 RegisterPlayerMsg。
type testReq struct{ UID uint64 }

func (r *testReq) GetUID() uint64 { return r.UID }

// spawnPlayerActor 启动一个只注册了初始化配置中路由的玩家角色。
func spawnPlayerActor(t *testing.T, boot func(*core.PlayerActor)) tree.PID {
	t.Helper()
	actor := &core.PlayerActor{
		UID:    testUID,
		Player: player.NewPlayerData(testUID),
		Boot:   boot,
	}
	pid := tree.SpawnOne(actor)
	t.Cleanup(func() { tree.Stop(pid) })
	return pid
}

// guildSaveSnapshot 读取玩家存盘所用的独立快照及其标脏状态。
type guildSaveSnapshot struct{}

// TestPlayerGuildMembershipIsPersisted 验证所属联盟由玩家 Actor 修改，并进入 BSON 存盘快照。
func TestPlayerGuildMembershipIsPersisted(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		playinternal.InitRoutes(actor)
		actor.GetPlayer().SetDirty(false)
		core.RegisterSysMsg(actor, (*guildSaveSnapshot)(nil), func(ctx *core.Context, _ *guildSaveSnapshot) {
			current := ctx.GetPlayer()
			ctx.Response(current.DeepCopy(), nil)
		})
	})

	for _, guildID := range []uint64{9, 0} {
		tree.Send(pid, &ipb.PlayerGuildMembership{UID: testUID, GuildID: guildID})
		value, err := tree.Request(pid, &guildSaveSnapshot{}).AwaitTimeout(time.Second)
		if err != nil {
			t.Fatalf("读取玩家存盘快照失败: %v", err)
		}
		snapshot := value.(*player.Player)
		if snapshot.GetGuildID() != guildID || !snapshot.IsDirty() {
			t.Fatalf("联盟编号未更新或未标脏，期望联盟编号 %v", guildID)
		}
		encoded, err := bson.Marshal(snapshot)
		if err != nil {
			t.Fatalf("编码玩家 BSON 失败: %v", err)
		}
		var stored struct {
			GuildID *uint64 `bson:"guild_id"`
		}
		if err := bson.Unmarshal(encoded, &stored); err != nil {
			t.Fatalf("读取玩家 BSON 失败: %v", err)
		}
		if stored.GuildID == nil || *stored.GuildID != guildID {
			t.Fatalf("玩家 BSON 中缺少正确的 guild_id，期望 %v", guildID)
		}
	}
}

// TestContextResponseReachesRequester 验证 Context.TreeCtx 使用当前消息的上下文。
func TestContextResponseReachesRequester(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Response(ctx.PlayerID(), nil)
		})
	})

	got, err := tree.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("Response 未回到请求方: %v", err)
	}
	if got != testUID {
		t.Fatalf("uid = %v, want %v", got, testUID)
	}
}

// TestRegisterPlayerInjectsPlayer 验证请求上下文通过玩家角色访问玩家数据。
func TestRegisterPlayerInjectsPlayer(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			if ctx.GetPlayer() == nil {
				ctx.Response(nil, errNoPlayer)
				return
			}
			ctx.Response(ctx.GetPlayer().GetUID(), nil)
		})
	})

	got, err := tree.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != testUID {
		t.Fatalf("player UID = %v, want %v", got, testUID)
	}
}

// TestRegisterPlayerRejectsAnotherUID 验证请求不能访问其他玩家角色的数据。
func TestRegisterPlayerRejectsAnotherUID(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterPlayerMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			t.Error("handler 不应被调用：请求 UID 不属于当前 Actor")
		})
	})

	if _, err := tree.Request(pid, &testReq{UID: testUID + 1}).AwaitTimeout(time.Second); err == nil {
		t.Fatal("UID 不匹配时应返回 error，实际为 nil")
	}
}

// TestRegisterSysUsesActorPlayer 验证系统消息也处理当前角色的玩家数据。
func TestRegisterSysUsesActorPlayer(t *testing.T) {
	pid := spawnPlayerActor(t, func(actor *core.PlayerActor) {
		core.RegisterSysMsg(actor, (*testReq)(nil), func(ctx *core.Context, _ *testReq) {
			ctx.Response(ctx.GetPlayer() != nil && ctx.PlayerID() == testUID, nil)
		})
	})

	got, err := tree.Request(pid, &testReq{UID: testUID}).AwaitTimeout(time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Fatal("RegisterSys 应注入当前 Actor 的玩家数据")
	}
}

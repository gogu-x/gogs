package internal

import (
	"time"

	"github.com/gogu-x/gogs/battle/battle/internal/configcompiler"
	"github.com/gogu-x/gogs/battle/battle/internal/engine"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// Mode 表示 BattleActor 的运行模式。
type Mode uint8

const (
	// ModeRun 创建并执行一场新战斗；零值默认使用此模式。
	ModeRun Mode = iota
	// ModeRedeliver 重新投递已保存的战斗结果。
	ModeRedeliver
)

type Params struct {
	BattleID      string                   // Manager 生成的战斗 ID
	Seed          uint64                   // Manager 生成的随机种子
	Request       *pb.StartBattleReq       // Game 权威组装的原始创建请求
	Compiler      *configcompiler.Compiler // 配置编译器；nil 时使用默认配置编译器
	UID           uint64                   // 发起玩家 ID
	Source        Source                   // 发起 Game 节点
	ManagerPID    tree.PID                 // Manager 的 PID
	Mode          Mode                     // Actor 运行模式
	Report        Report                   // 重投模式的已有战报
	Reports       Repository               // 战报持久化接口
	Notifier      Notifier                 // 生命周期通知接口
	RetryDelay    time.Duration            // Finished 重试延迟
	RetryAttempts int                      // Finished 最大重试次数
	Battle        *engine.Battle           // 仅重投或测试时可预置的引擎
	// PaceTickDuration 覆盖每个 tick 的真实墙钟时长。
	// 0 表示用引擎的 TickDurationMS；负数表示不节流（一次推完），仅供测试使用。
	PaceTickDuration time.Duration
}

// RetireBattle 请求 Actor 主动退出。
type RetireBattle struct{}

// battleTick 是节拍器的到期消息，由时间轮投递回 Actor 自己的邮箱。
type battleTick struct{}

// ActorStopped 表示 BattleActor 已结束，可由 Manager 清理 active 登记。
type ActorStopped struct {
	BattleID string   // 已停止的战斗 ID
	PID      tree.PID // 已停止 Actor 的 PID
}

type retryTick struct{}

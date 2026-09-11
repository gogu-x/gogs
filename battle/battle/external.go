package battle

import (
	"github.com/gogu-x/gogs/battle/battle/internal"
	"github.com/gogu-x/tree"
)

type Params = internal.Params
type Mode = internal.Mode

const (
	ModeRun       = internal.ModeRun
	ModeRedeliver = internal.ModeRedeliver
)

type Source = internal.Source
type Notifier = internal.Notifier
type NotifyFunc = internal.NotifyFunc
type Repository = internal.Repository
type ActorStopped = internal.ActorStopped

// New 创建一场独立运行的 BattleActor。
func New(p Params) tree.Actor {
	return internal.NewBattleActor(p)
}

// NewMemoryRepository 创建本地运行和测试可用的战报仓储。
func NewMemoryRepository() Repository { return internal.NewMemoryRepository() }

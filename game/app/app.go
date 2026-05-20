package app

import (
	"github.com/gogu-x/gogs/game/activity"
	"github.com/gogu-x/gogs/game/guild"
	"github.com/gogu-x/gogs/game/player"
)

type App struct {
	Players  *player.Mgr
	Guild    *guild.Manager
	Activity *activity.Manager
}

func New() *App {
	return &App{
		Players: player.NewManager(),
	}
}

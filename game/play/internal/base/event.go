package base

import (
	"github.com/gogu-x/gogs/game/play/internal/module/asset"
)

const (
	PlayerOnLogin = "PlayerOnLogin"
)

func InitEven(app *App) {
	app.event.Register(PlayerOnLogin, asset.OnLogin, 0)
}

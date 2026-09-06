package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/context"
	"github.com/gogu-x/gogs/game/play/internal/module/asset"
)

func InitEvent(app *Play) {
	app.event.Register(context.PlayerOnLogin, asset.OnLogin, 0)
}

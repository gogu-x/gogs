package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/gogs/game/play/internal/module/asset"
)

func InitEvent(app *Play) {
	app.event.Register(common.PlayerOnLogin, asset.OnLogin, 0)
}

package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_chat"
)

func InitEvent(py *core.Play) {
	py.OnEvent(core.PlayerOnLogin, ctl_chat.OnLogin, 0)
}

package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_chat"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_tower"
)

func InitEvent(py *core.Play) {
	py.OnEvent(core.PlayerOnLogin, ctl_chat.OnLogin, 0)
	py.OnEvent(core.PlayerOnLogout, ctl_auth.OnLogout, 0)
	py.OnEvent(core.BattleSettled, ctl_tower.OnTowerBattleResults, 0)

}

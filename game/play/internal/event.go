package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_chat"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_tower"
)

// InitEvent 注册当前玩家角色的业务事件监听器。
func InitEvent(actor *core.PlayerActor) {
	actor.OnEvent(core.PlayerOnLogin, ctl_chat.OnLogin, 0)
	actor.OnEvent(core.PlayerOnLogout, ctl_auth.OnLogout, 0)
	actor.OnEvent(core.BattleSettled, ctl_tower.OnTowerBattleResults, 0)
}

package app

import (
	"github.com/gogu-x/gogs/game/player"
	"github.com/gogu-x/gogs/game/player/bag"
	"github.com/gogu-x/gogs/game/player/cardGroup"
	"github.com/gogu-x/gogs/game/player/shop"
)

type App struct {
	Player       *player.Player
	ConnID       uint64
	GateId       string
	BagMgr       *bag.Mgr
	ShopMgr      *shop.Mgr
	CardGroupMgr *cardGroup.Mgr
}

func New(uid uint64) *App {
	return &App{
		Player: &player.Player{UID: uid},
	}
}

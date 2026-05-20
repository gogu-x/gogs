package card_group

import "github.com/gogu-x/gogs/game/player"

func AddCardGroup(pId uint64) {
	player.NewManager().Get(pId)
}

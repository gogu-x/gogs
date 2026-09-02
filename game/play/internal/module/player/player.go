package player

import (
	"github.com/gogu-x/gogs/game/play/internal/module/asset"
	"github.com/gogu-x/gogs/game/play/internal/module/bag"
	"github.com/gogu-x/gogs/game/play/internal/module/cardgroup"
	"github.com/gogu-x/gogs/game/play/internal/module/shop"
)

type Player struct {
	UID   uint64 `bson:"_id"`
	Name  string `bson:"name"`
	Level uint32 `bson:"level"`
	State int    `bson:"state"`

	AssetMgr     *asset.Mgr     `bson:"asset"`
	BagMgr       *bag.Mgr       `bson:"bag"`
	CardGroupMgr *cardgroup.Mgr `bson:"card_group"`
	ShopMgr      *shop.Mgr      `bson:"shop"`
}

func NewPlayerData(uid uint64) *Player {

	return &Player{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		ShopMgr:      &shop.Mgr{},
	}
}

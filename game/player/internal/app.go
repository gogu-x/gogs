package internal

import (
	"github.com/gogu-x/gogs/game/player/module/asset"
	"github.com/gogu-x/gogs/game/player/module/bag"
	"github.com/gogu-x/gogs/game/player/module/cardgroup"
	"github.com/gogu-x/gogs/game/player/module/shop"
)

// PlayerData 玩家全量数据，随 PlayerActor 生命周期存活
type PlayerData struct {
	UID   uint64
	Name  string
	Level uint32
	State int

	AssetMgr     *asset.Mgr
	BagMgr       *bag.Mgr
	CardGroupMgr *cardgroup.Mgr
	ShopMgr      *shop.Mgr
}

func NewPlayerData(uid uint64) *PlayerData {
	return &PlayerData{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		ShopMgr:      &shop.Mgr{},
	}
}

// Load 从 DB 加载玩家数据（TODO）
func Load(uid uint64) *PlayerData {
	return NewPlayerData(uid)
}

// Save 将玩家数据持久化到 DB（TODO）
func (p *PlayerData) Save() error {
	return nil
}

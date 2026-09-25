package player

import (
	"fmt"

	"github.com/gogu-x/gogs/game/play/internal/module/player/asset"
	"github.com/gogu-x/gogs/game/play/internal/module/player/bag"
	"github.com/gogu-x/gogs/game/play/internal/module/player/cardgroup"
	"github.com/gogu-x/gogs/game/play/internal/module/player/equipment"
	"github.com/gogu-x/gogs/game/play/internal/module/player/role"
	"github.com/gogu-x/gogs/game/play/internal/module/player/shop"
	"github.com/gogu-x/gogs/game/play/internal/module/player/tower"
	"github.com/gogu-x/gogs/glconf"
)

type Player struct {
	UID   uint64 `bson:"_id"`
	Name  string `bson:"name"`
	Level uint32 `bson:"level"`
	State int    `bson:"state"`

	AssetMgr     *asset.Mgr     `bson:"asset"`
	BagMgr       *bag.Mgr       `bson:"bag"`
	CardGroupMgr *cardgroup.Mgr `bson:"card_group"`
	RoleMgr      *role.Mgr      `bson:"role"`
	EquipmentMgr *equipment.Mgr `bson:"equipment"`
	ShopMgr      *shop.Mgr      `bson:"shop"`
	TowerMgr     *tower.Mgr     `bson:"tower"`
}

func NewPlayerData(uid uint64) *Player {
	player := &Player{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		RoleMgr:      role.New(),
		EquipmentMgr: equipment.New(),
		ShopMgr:      &shop.Mgr{},
		TowerMgr:     tower.NewTowerMgr(),
	}
	player.initDefaultCharacter()
	return player
}

func (p *Player) initDefaultCharacter() {
	if p == nil || p.RoleMgr == nil {
		return
	}
	config := glconf.GetPlayerInitRoleCfg(1)
	if config == nil {
		return
	}
	roleID := fmt.Sprintf("%d", p.UID)
	p.RoleMgr.Character = &role.Role{
		ID:           roleID,
		ConfigID:     config.RoleConfigID,
		Level:        config.Level,
		Star:         config.Star,
		Breakthrough: config.Breakthrough,
	}
}

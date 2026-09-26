//go:generate go run ../../../../../tools/deepcopy-gen -dir .

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

// Player 保存玩家持久化数据和运行期各模块数据。
// +deepcopy-gen=true
type Player struct {
	UID   uint64 `bson:"_id"`   // 玩家唯一 ID。
	Name  string `bson:"name"`  // 玩家名称。
	Level uint32 `bson:"level"` // 玩家等级。
	State int    `bson:"state"` // 玩家状态。

	AssetMgr     *asset.Mgr     `bson:"asset"`      // 资产模块数据。
	BagMgr       *bag.Mgr       `bson:"bag"`        // 背包模块数据。
	CardGroupMgr *cardgroup.Mgr `bson:"card_group"` // 卡组模块数据。
	RoleMgr      *role.Mgr      `bson:"role"`       // 角色模块数据。
	EquipmentMgr *equipment.Mgr `bson:"equipment"`  // 装备模块数据。
	ShopMgr      *shop.Mgr      `bson:"shop"`       // 商店模块数据。
	TowerMgr     *tower.Mgr     `bson:"tower"`      // 塔模块数据。
	TestMsgMap   map[int]int32  `bson:"-"`

	Dirty bool
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
	player.initStarterEquipment()
	return player
}

// GetUID 返回玩家唯一 ID。
func (p *Player) GetUID() uint64 {
	if p == nil {
		return 0
	}
	return p.UID
}

// GetRoleMgr 返回玩家角色模块。
func (p *Player) GetRoleMgr() *role.Mgr {
	if p == nil {
		return nil
	}
	return p.RoleMgr
}

// GetEquipmentMgr 返回玩家装备模块。
func (p *Player) GetEquipmentMgr() *equipment.Mgr {
	if p == nil {
		return nil
	}
	return p.EquipmentMgr
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

// initStarterEquipment gives a new character one item per slot plus one alternative,
// so the equipment screen is immediately usable in development and new accounts.
func (p *Player) initStarterEquipment() {
	if p == nil || p.EquipmentMgr == nil || p.RoleMgr == nil || p.RoleMgr.Character == nil {
		return
	}
	if p.EquipmentMgr.Items == nil {
		p.EquipmentMgr.Items = make(map[string]*equipment.Item)
	}
	if len(p.EquipmentMgr.Items) > 0 {
		return
	}
	for configID := int32(1); configID <= 12; configID++ {
		slot := (configID-1)%6 + 1
		id := fmt.Sprintf("%d-gear-%02d", p.UID, configID)
		p.EquipmentMgr.Items[id] = &equipment.Item{
			ID: id, ConfigID: configID, Slot: slot, Level: 1,
		}
		if configID <= 6 {
			p.RoleMgr.Character.EquipmentIDs = append(p.RoleMgr.Character.EquipmentIDs, id)
		}
	}
}

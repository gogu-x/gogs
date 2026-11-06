//go:generate make -C ../../../.. deepcopy GEN_DIR=game

package player

import (
	"fmt"

	"github.com/gogu-x/gogs/game/play/internal/player/asset"
	"github.com/gogu-x/gogs/game/play/internal/player/bag"
	"github.com/gogu-x/gogs/game/play/internal/player/cardgroup"
	"github.com/gogu-x/gogs/game/play/internal/player/equipment"
	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/game/play/internal/player/shop"
	"github.com/gogu-x/gogs/game/play/internal/player/souljade"
	"github.com/gogu-x/gogs/game/play/internal/player/talent"
	"github.com/gogu-x/gogs/game/play/internal/player/tower"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/tlog"
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
	SoulJadeMgr  *souljade.Mgr  `bson:"soul_jade"`  // Owned soul jades and slot assignments.
	TalentMgr    *talent.Mgr    `bson:"talent"`     // Learned talent state.
	ShopMgr      *shop.Mgr      `bson:"shop"`       // 商店模块数据。
	TowerMgr     *tower.Mgr     `bson:"tower"`      // 塔模块数据。
	TestMsgMap   map[int]int32  `bson:"-"`          // 测试期间使用的临时消息数据。

	Dirty    bool   // 玩家数据是否需要存盘。
	revision uint64 // 内存版本号，不写入玩家文档。
}

func NewPlayerData(uid uint64) *Player {
	player := &Player{
		UID:          uid,
		AssetMgr:     &asset.Mgr{},
		BagMgr:       &bag.Mgr{},
		CardGroupMgr: cardgroup.New(),
		RoleMgr:      role.New(),
		EquipmentMgr: equipment.New(),
		SoulJadeMgr:  souljade.New(uid),
		TalentMgr:    talent.New(),
		ShopMgr:      &shop.Mgr{},
		TowerMgr:     tower.NewTowerMgr(),
	}
	player.initDefaultCharacter()
	player.initStarterEquipment()
	player.MarkDirty()
	return player
}

// MarkDirty 标记玩家数据已修改，并递增内存版本号。
func (p *Player) MarkDirty() {
	if p == nil {
		return
	}
	p.SetDirty(true)
}

// Revision 返回玩家数据的内存版本号。
func (p *Player) Revision() uint64 {
	if p == nil {
		return 0
	}
	return p.revision
}

// IsDirty 返回玩家数据是否需要存盘。
func (p *Player) IsDirty() bool {
	return p != nil && p.Dirty
}

// SetDirty 更新玩家数据的存盘标记。
func (p *Player) SetDirty(dirty bool) {
	if p == nil {
		return
	}
	if dirty {
		p.revision++
	}
	p.Dirty = dirty
}

// GetUID 返回玩家唯一编号。
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

// GetTowerMgr 返回玩家爬塔模块。
func (p *Player) GetTowerMgr() *tower.Mgr {
	if p == nil {
		return nil
	}
	return p.TowerMgr
}

// GetSoulJadeMgr returns the owned soul jade module.
func (p *Player) GetSoulJadeMgr() *souljade.Mgr {
	if p == nil {
		return nil
	}
	return p.SoulJadeMgr
}

// GetTalentMgr returns the owned talent module.
func (p *Player) GetTalentMgr() *talent.Mgr {
	if p == nil {
		return nil
	}
	return p.TalentMgr
}

// EnsureLoadoutModules initializes missing modules once during login loading.
func (p *Player) EnsureLoadoutModules() bool {
	if p == nil {
		return false
	}
	changed := false
	if p.GetSoulJadeMgr() == nil {
		if module := souljade.New(p.GetUID()); module != nil {
			p.SoulJadeMgr = module
			changed = true
		}
	}
	if p.GetTalentMgr() == nil {
		if module := talent.New(); module != nil {
			p.TalentMgr = module
			changed = true
		}
	}
	if souljade.NormalizeSlots(p.GetSoulJadeMgr()) {
		changed = true
	}
	if talent.EnsureRoots(p.GetTalentMgr()) {
		changed = true
	}
	if changed {
		p.MarkDirty()
	}
	return changed
}

func (p *Player) initDefaultCharacter() {
	if p == nil {
		return
	}
	roleMgr := p.GetRoleMgr()
	if roleMgr == nil {
		return
	}
	config := glconf.GetPlayerInitRoleCfg(1)
	if config == nil {
		return
	}
	roleID := fmt.Sprintf("%d", p.GetUID())
	roleMgr.SetCharacter(&role.Role{
		ID:           roleID,
		ConfigID:     config.GetRoleConfigID(),
		Level:        config.GetLevel(),
		Star:         config.GetStar(),
		Breakthrough: config.GetBreakthrough(),
	})
}

// initStarterEquipment initializes inventory and equipped items from the role profile.
func (p *Player) initStarterEquipment() {
	if p == nil {
		return
	}
	equipmentMgr := p.GetEquipmentMgr()
	roleMgr := p.GetRoleMgr()
	if equipmentMgr == nil || roleMgr == nil {
		return
	}
	character, ok := roleMgr.GetCharacter()
	if !ok || character == nil {
		return
	}
	if len(equipmentMgr.GetItems()) > 0 {
		return
	}
	items, equippedIDs, code := equipment.InitialLoadout(p.GetUID(), glconf.GetPlayerInitRoleCfg(1))
	if code != pb_common.ErrCode_OK {
		tlog.Log.Error("[player/initStarterEquipment] 初始装备配置无效, pid=%v code=%v", p.GetUID(), code)
		return
	}
	for _, item := range items {
		equipmentMgr.AddItem(item)
	}
	character.SetEquipmentIDs(equippedIDs)
}

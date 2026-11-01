package equipment

import (
	"fmt"

	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

// InitialLoadout builds configured starter instances without mutating a player.
func InitialLoadout(uid uint64, profile *glconf.PlayerInitRoleCfg) ([]*Item, []string, pb_common.ErrCode) {
	if profile == nil {
		return nil, nil, pb_common.ErrCode_INTERNAL
	}
	configIDs := profile.GetEquipmentConfigIDs()
	items := make([]*Item, 0, len(configIDs))
	byConfig := make(map[int32]*Item, len(configIDs))
	for _, configID := range configIDs {
		config := glconf.GetBattleEquipCfg(configID)
		if config == nil || glconf.GetEquipmentSlotCfg(config.GetSlot()) == nil ||
			config.GetInitialLevel() <= 0 || config.GetInitialRefineLevel() < 0 || byConfig[configID] != nil {
			return nil, nil, pb_common.ErrCode_INTERNAL
		}
		item := &Item{
			ID: fmt.Sprintf("%d-gear-%02d", uid, configID), ConfigID: configID,
			Slot: config.GetSlot(), Level: config.GetInitialLevel(), RefineLevel: config.GetInitialRefineLevel(),
		}
		items = append(items, item)
		byConfig[configID] = item
	}
	configuredEquipped := profile.GetEquippedConfigIDs()
	equippedIDs := make([]string, 0, len(configuredEquipped))
	usedSlots := make(map[int32]bool, len(configuredEquipped))
	for _, configID := range configuredEquipped {
		item := byConfig[configID]
		if item == nil || usedSlots[item.GetSlot()] {
			return nil, nil, pb_common.ErrCode_INTERNAL
		}
		usedSlots[item.GetSlot()] = true
		equippedIDs = append(equippedIDs, item.GetID())
	}
	return items, equippedIDs, pb_common.ErrCode_OK
}

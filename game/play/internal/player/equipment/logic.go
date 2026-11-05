package equipment

import (
	"sort"

	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
)

// ChangeRoleEquipment validates configured slots and updates the in-memory loadout.
func ChangeRoleEquipment(
	roleMgr *role.Mgr,
	equipmentMgr *Mgr,
	slot int32,
	equipmentID string,
) pb_common.ErrCode {
	if glconf.GetEquipmentSlotCfg(slot) == nil {
		return pb_common.ErrCode_PARAM
	}
	character, ok := roleMgr.GetCharacter()
	if !ok || equipmentMgr == nil {
		return pb_common.ErrCode_PARAM
	}
	if equipmentID != "" {
		item, exists := equipmentMgr.Get(equipmentID)
		if !exists {
			return pb_common.ErrCode_PARAM
		}
		config := glconf.GetBattleEquipCfg(item.GetConfigID())
		if config == nil {
			return pb_common.ErrCode_INTERNAL
		}
		if config.GetSlot() != slot {
			return pb_common.ErrCode_PARAM
		}
	}
	currentIDs := character.GetEquipmentIDs()
	updatedIDs := make([]string, 0, len(currentIDs)+1)
	for _, currentID := range currentIDs {
		item, exists := equipmentMgr.Get(currentID)
		if !exists {
			updatedIDs = append(updatedIDs, currentID)
			continue
		}
		config := glconf.GetBattleEquipCfg(item.GetConfigID())
		if config == nil {
			return pb_common.ErrCode_INTERNAL
		}
		if config.GetSlot() != slot {
			updatedIDs = append(updatedIDs, currentID)
		}
	}
	if equipmentID != "" {
		updatedIDs = append(updatedIDs, equipmentID)
	}
	character.SetEquipmentIDs(updatedIDs)
	return pb_common.ErrCode_OK
}

// BuildRoleEquipmentData joins owned instances with the current configuration catalog.
func BuildRoleEquipmentData(
	roleMgr *role.Mgr,
	equipmentMgr *Mgr,
) (*pb_equipment.RoleEquipmentData, pb_common.ErrCode) {
	character, ok := roleMgr.GetCharacter()
	if !ok || equipmentMgr == nil {
		return nil, pb_common.ErrCode_PARAM
	}
	equipped := make(map[string]struct{})
	for _, equipmentID := range character.GetEquipmentIDs() {
		equipped[equipmentID] = struct{}{}
	}
	items := equipmentMgr.GetItems()
	slots := make([]*pb_equipment.EquipmentSlotDefinition, 0)
	for _, slot := range glconf.GetAllEquipmentSlotCfg() {
		slots = append(slots, &pb_equipment.EquipmentSlotDefinition{
			Slot: pb_equipment.EquipSlot(slot.GetCfgID()), Name: slot.GetName(), IconKey: slot.GetIconKey(),
		})
	}
	if len(slots) == 0 {
		return nil, pb_common.ErrCode_INTERNAL
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].GetSlot() < slots[j].GetSlot() })
	data := &pb_equipment.RoleEquipmentData{
		RoleId: character.GetID(), RoleLevel: character.GetLevel(), Slots: slots,
	}
	entries := make([]*pb_equipment.EquipmentItem, 0, len(items))
	for _, item := range items {
		configID := item.GetConfigID()
		config := glconf.GetBattleEquipCfg(configID)
		if config == nil || glconf.GetEquipmentSlotCfg(config.GetSlot()) == nil || config.GetName() == "" {
			return nil, pb_common.ErrCode_INTERNAL
		}
		itemID := item.GetID()
		entry := &pb_equipment.EquipmentItem{
			InstanceId:  itemID,
			ConfigId:    configID,
			Slot:        pb_equipment.EquipSlot(config.GetSlot()),
			Level:       item.GetLevel(),
			RefineLevel: item.GetRefineLevel(),
			DisplayName: config.GetName(),
			Rarity:      config.GetRarity(),
			IconKey:     config.GetIconKey(),
		}
		_, entry.Equipped = equipped[itemID]
		for _, modifier := range config.GetModifier() {
			if modifier == nil || modifier.GetTyp() != "battle_attribute" {
				continue
			}
			switch modifier.GetId() {
			case 1:
				entry.MaxHp += modifier.GetVal()
			case 2:
				entry.Attack += modifier.GetVal()
			case 3:
				entry.Defense += modifier.GetVal()
			case 4:
				entry.Speed += modifier.GetVal()
			}
		}
		if entry.GetEquipped() {
			data.MaxHp += entry.GetMaxHp()
			data.Attack += entry.GetAttack()
			data.Defense += entry.GetDefense()
			data.Speed += entry.GetSpeed()
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.GetSlot() != right.GetSlot() {
			return left.GetSlot() < right.GetSlot()
		}
		return left.GetConfigId() < right.GetConfigId()
	})
	data.Items = entries
	return data, pb_common.ErrCode_OK
}

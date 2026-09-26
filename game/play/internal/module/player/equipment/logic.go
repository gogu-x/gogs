package equipment

import (
	"sort"

	"github.com/gogu-x/gogs/game/play/internal/module/player/role"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
)

// ChangeRoleEquipment 校验并更新角色内存中的装备穿戴关系。
func ChangeRoleEquipment(
	roleMgr *role.Mgr,
	equipmentMgr *Mgr,
	slot int32,
	equipmentID string,
) pb_common.ErrCode {
	if slot < 1 || slot > 6 {
		return pb_common.ErrCode_PARAM
	}
	character, ok := roleMgr.GetCharacter()
	if !ok || equipmentMgr == nil {
		return pb_common.ErrCode_PARAM
	}
	if equipmentID != "" {
		item, exists := equipmentMgr.Get(equipmentID)
		if !exists || item.GetSlot() != slot {
			return pb_common.ErrCode_PARAM
		}
	}
	currentIDs := character.GetEquipmentIDs()
	updatedIDs := make([]string, 0, len(currentIDs)+1)
	for _, currentID := range currentIDs {
		item, exists := equipmentMgr.Get(currentID)
		if !exists || item.GetSlot() != slot {
			updatedIDs = append(updatedIDs, currentID)
		}
	}
	if equipmentID != "" {
		updatedIDs = append(updatedIDs, equipmentID)
	}
	character.SetEquipmentIDs(updatedIDs)
	return pb_common.ErrCode_OK
}

// BuildRoleEquipmentData 根据玩家内存数据组装客户端装备视图。
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
	sort.Slice(items, func(i, j int) bool {
		if items[i].GetSlot() != items[j].GetSlot() {
			return items[i].GetSlot() < items[j].GetSlot()
		}
		return items[i].GetConfigID() < items[j].GetConfigID()
	})
	data := &pb_equipment.RoleEquipmentData{
		RoleId:    character.GetID(),
		RoleLevel: character.GetLevel(),
	}
	for _, item := range items {
		configID := item.GetConfigID()
		config := glconf.GetBattleEquipCfg(configID)
		if config == nil {
			return nil, pb_common.ErrCode_PARAM
		}
		itemID := item.GetID()
		entry := &pb_equipment.EquipmentItem{
			InstanceId:  itemID,
			ConfigId:    configID,
			Slot:        pb_equipment.EquipSlot(item.GetSlot()),
			Level:       item.GetLevel(),
			RefineLevel: item.GetRefineLevel(),
		}
		_, entry.Equipped = equipped[itemID]
		for _, modifier := range config.Modifier {
			if modifier == nil || modifier.Typ != "battle_attribute" {
				continue
			}
			switch modifier.Id {
			case 1:
				entry.MaxHp += modifier.Val
			case 2:
				entry.Attack += modifier.Val
			case 3:
				entry.Defense += modifier.Val
			case 4:
				entry.Speed += modifier.Val
			}
		}
		if entry.Equipped {
			data.MaxHp += entry.MaxHp
			data.Attack += entry.Attack
			data.Defense += entry.Defense
			data.Speed += entry.Speed
		}
		data.Items = append(data.Items, entry)
	}
	return data, pb_common.ErrCode_OK
}

package equipment

import (
	"testing"

	"github.com/gogu-x/gogs/game/play/internal/player/role"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

func configuredEquipment(t *testing.T) {
	t.Helper()
	previousSlots := glconf.GetAllEquipmentSlotCfg()
	previousItems := glconf.GetAllBattleEquipCfg()
	t.Cleanup(func() {
		_ = glconf.SetEquipmentSlotCfgForTest(previousSlots)
		_ = glconf.SetBattleEquipCfgForTest(previousItems)
	})
	if err := glconf.SetEquipmentSlotCfgForTest([]*glconf.EquipmentSlotCfg{
		{CfgID: 17, Name: "Configured armor slot", IconKey: "robe"},
		{CfgID: 42, Name: "Configured weapon slot", IconKey: "sword"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetBattleEquipCfgForTest([]*glconf.BattleEquipCfg{
		{CfgID: 77, Slot: 17, Name: "Configured armor", Rarity: "Rare", IconKey: "robe", InitialLevel: 4, InitialRefineLevel: 2,
			Modifier: []*pb_common.TypIDVal{{Typ: "battle_attribute", Id: 1, Val: 25}, {Typ: "battle_attribute", Id: 2, Val: 7}}},
		{CfgID: 78, Slot: 17, Name: "Alternate armor", InitialLevel: 2},
		{CfgID: 91, Slot: 42, Name: "Configured sword", InitialLevel: 3},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInitialEquipmentUsesProfileAndCatalog(t *testing.T) {
	configuredEquipment(t)
	profile := &glconf.PlayerInitRoleCfg{EquipmentConfigIDs: []int32{91, 77}, EquippedConfigIDs: []int32{77}}
	items, equipped, code := InitialLoadout(8, profile)
	if code != pb_common.ErrCode_OK || len(items) != 2 || len(equipped) != 1 {
		t.Fatal("configured inventory was not initialized")
	}
	if items[0].GetConfigID() != 91 || items[0].GetSlot() != 42 || items[0].GetLevel() != 3 {
		t.Fatal("starter inventory still assumes consecutive IDs or fixed slots")
	}
	if items[1].GetLevel() != 4 || items[1].GetRefineLevel() != 2 || equipped[0] != items[1].GetID() {
		t.Fatal("configured initial progression or equipped selection was ignored")
	}
}

func TestInvalidStarterConfigurationDoesNotPartiallyGrant(t *testing.T) {
	configuredEquipment(t)
	profiles := []*glconf.PlayerInitRoleCfg{
		nil,
		{EquipmentConfigIDs: []int32{77, 999}},
		{EquipmentConfigIDs: []int32{77, 77}},
		{EquipmentConfigIDs: []int32{77}, EquippedConfigIDs: []int32{91}},
		{EquipmentConfigIDs: []int32{77, 78}, EquippedConfigIDs: []int32{77, 78}},
	}
	for _, profile := range profiles {
		items, equipped, code := InitialLoadout(8, profile)
		if code == pb_common.ErrCode_OK || items != nil || equipped != nil {
			t.Fatal("invalid starter configuration produced a partial grant")
		}
	}
}

func TestEquipmentRulesAndPresentationUseConfiguration(t *testing.T) {
	configuredEquipment(t)
	roles, inventory := role.New(), New()
	roles.SetCharacter(&role.Role{ID: "test-role", ConfigID: 1, Level: 1})
	inventory.AddItem(&Item{ID: "test-armor", ConfigID: 77, Slot: 1, Level: 9})
	if code := ChangeRoleEquipment(roles, inventory, 1, "test-armor"); code == pb_common.ErrCode_OK {
		t.Fatal("stale persisted slot was trusted instead of configuration")
	}
	if code := ChangeRoleEquipment(roles, inventory, 17, "test-armor"); code != pb_common.ErrCode_OK {
		t.Fatal("configured slot outside the old fixed range was rejected")
	}
	data, code := BuildRoleEquipmentData(roles, inventory)
	if code != pb_common.ErrCode_OK || len(data.GetItems()) != 1 || len(data.GetSlots()) != 2 {
		t.Fatal("configured catalog is missing from the snapshot")
	}
	item := data.GetItems()[0]
	if item.GetDisplayName() != "Configured armor" || item.GetRarity() != "Rare" || item.GetIconKey() != "robe" || int32(item.GetSlot()) != 17 {
		t.Fatal("equipment presentation does not match configuration")
	}
	if item.GetLevel() != 9 || data.GetMaxHp() != 25 || data.GetAttack() != 7 {
		t.Fatal("instance progression or configured attribute modifiers were lost")
	}
	if code := ChangeRoleEquipment(roles, inventory, 17, ""); code != pb_common.ErrCode_OK {
		t.Fatal("configured slot cannot be unequipped")
	}
	data, _ = BuildRoleEquipmentData(roles, inventory)
	if data.GetMaxHp() != 0 || data.GetAttack() != 0 {
		t.Fatal("unequipped bonuses remain active")
	}
}

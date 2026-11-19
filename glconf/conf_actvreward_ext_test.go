package glconf

import (
	"math"
	"sync"
	"testing"
)

func TestShopRewardGroups(t *testing.T) {
	InitGameCfg()
	previousConfs := confs
	previousCallbacks := afterLoadCallbacks
	previousRegister := registerConfigCallbacksOnce
	previousRewards, _ := csvActvRewardCfg.Load().(*CsvConf)
	previousEquipment, _ := csvBattleEquipCfg.Load().(*CsvConf)
	previousGroups, _ := shopRewardGroups.Load().(shopRewardIndex)
	t.Cleanup(func() {
		confs = previousConfs
		afterLoadCallbacks = previousCallbacks
		registerConfigCallbacksOnce = previousRegister
		csvActvRewardCfg.Store(previousRewards)
		csvBattleEquipCfg.Store(previousEquipment)
		shopRewardGroups.Store(previousGroups)
	})
	afterLoadCallbacks = make(map[string][]func(*CsvConf))
	registerConfigCallbacksOnce = sync.OnceFunc(registerConfigCallbacks)
	csvActvRewardCfg.Store((*CsvConf)(nil))
	csvBattleEquipCfg.Store((*CsvConf)(nil))
	shopRewardGroups.Store(shopRewardIndex(nil))

	rewards := []*ActvRewardCfg{
		{CfgID: 1, GroupID: 10, RewardType: 1, RewardID: 100, Count: 3},
		{CfgID: 2, GroupID: 10, RewardType: 2, RewardID: 200, Count: 1000},
		{CfgID: 3, GroupID: 10, RewardType: 2, RewardID: 201, Count: 4000},
		{CfgID: 4, GroupID: 20, RewardType: 1, RewardID: 100, Count: math.MaxInt64},
		{CfgID: 5, GroupID: 30, RewardType: 1, RewardID: 100, Count: 1},
		{CfgID: 6, GroupID: 30, RewardType: 2, RewardID: 200, Count: math.MaxInt64},
	}
	equipment := []*BattleEquipCfg{{CfgID: 200}, {CfgID: 201}}
	loadRewards := func(_, _, _ string) (*CsvConf, error) {
		table, err := newConfigForTest(&ActvRewardCfg{}, interfacesOf(rewards))
		if err == nil {
			csvActvRewardCfg.Store(table)
		}
		return table, err
	}
	loadEquipment := func(_, _, _ string) (*CsvConf, error) {
		if asCsv(csvBattleEquipCfg) == nil && HasShopRewardGroup(20) {
			t.Fatal("published an item reward group before the equipment table loaded")
		}
		table, err := newConfigForTest(&BattleEquipCfg{}, interfacesOf(equipment))
		if err == nil {
			csvBattleEquipCfg.Store(table)
		}
		return table, err
	}
	confs = []confMeta{
		{source: ActvRewardCfgKey, loader: loadRewards},
		{source: BattleEquipCfgKey, loader: loadEquipment},
	}
	if err := LoadAllConfs("", "", false); err != nil {
		t.Fatal(err)
	}

	snapshot, ok := ShopRewards(10, 2)
	if !ok || len(snapshot) != 3 {
		t.Fatalf("expected three rewards, got %v", snapshot)
	}
	for i, want := range []int64{6, 2000, 8000} {
		if reward := snapshot[i]; reward.GetCount() != want || reward.GetID() != rewards[i].GetRewardID() {
			t.Fatalf("reward at index %d = %v, want ordered reward with count %d", i, reward, want)
		}
	}
	snapshot[0].SetCount(99)
	if next, ok := ShopRewards(10, 2); !ok || next[0].GetCount() != 6 {
		t.Fatalf("new purchase reused a mutated snapshot: %v", next)
	}
	if _, ok := ShopRewards(10, 3); ok {
		t.Fatal("allowed a purchase above the combined equipment limit")
	}
	if _, ok := ShopRewards(20, 2); ok {
		t.Fatal("allowed a purchase with an overflowing quantity")
	}
	if HasShopRewardGroup(30) {
		t.Fatal("published part of an invalid reward group")
	}

	equipment = equipment[:1]
	if err := LoadConfs([]string{BattleEquipCfgKey}, "", "", true); err != nil {
		t.Fatal(err)
	}
	if HasShopRewardGroup(10) || !HasShopRewardGroup(20) {
		t.Fatal("equipment reload did not invalidate only the affected reward group")
	}
	if snapshot[2].GetCount() != 8000 {
		t.Fatal("configuration reload changed an existing purchase snapshot")
	}

	rewardCallbacks := len(afterLoadCallbacks[ActvRewardCfgKey])
	equipmentCallbacks := len(afterLoadCallbacks[BattleEquipCfgKey])
	if err := LoadAllConfs("", "", true); err != nil {
		t.Fatal(err)
	}
	if len(afterLoadCallbacks[ActvRewardCfgKey]) != rewardCallbacks ||
		len(afterLoadCallbacks[BattleEquipCfgKey]) != equipmentCallbacks {
		t.Fatal("full reload registered duplicate callbacks")
	}
}

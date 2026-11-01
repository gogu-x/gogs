package souljade

import (
	"github.com/gogu-x/gogs/game/play/internal/player/loadouttest"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"testing"
)

func TestGenericSlotsRejectDuplicateDefinition(t *testing.T) {
	loadouttest.Load(t)
	mgr := New(100)
	items, slots := mgr.GetItems(), mgr.GetSlots()
	original := items[slots[1]]
	duplicate := original.DeepCopy()
	duplicate.SetID("duplicate-instance")
	items[duplicate.GetID()] = duplicate
	before := mgr.GetRevision()
	if code := Change(mgr, 6, duplicate.GetID()); code != pb_common.ErrCode_DUPLICATE_JADE {
		t.Fatal("another instance of the same definition must be rejected")
	}
	if mgr.GetRevision() != before {
		t.Fatal("rejected request changed revision")
	}
	if code := Change(mgr, 1, ""); code != pb_common.ErrCode_OK {
		t.Fatal("unequip failed")
	}
	if code := Change(mgr, 6, duplicate.GetID()); code != pb_common.ErrCode_OK {
		t.Fatal("generic slots must accept every configured definition")
	}
	if code := Change(mgr, 7, duplicate.GetID()); code == pb_common.ErrCode_OK {
		t.Fatal("invalid slot accepted")
	}
	if code := Change(mgr, 1, "other-player-jade"); code == pb_common.ErrCode_OK {
		t.Fatal("unowned instance accepted")
	}
}

func TestSoulJadeRulesComeFromConfiguration(t *testing.T) {
	loadouttest.Load(t)
	t.Cleanup(func() { loadouttest.Load(t) })
	if err := glconf.SetSoulJadeCfgForTest([]*glconf.SoulJadeCfg{{CfgID: 77, Name: "configured-jade", Rarity: "configured-rarity", IconKey: "configured-icon", InitialAttribute: 4, InitialPermille: 9}}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetLoadoutInitCfgForTest([]*glconf.LoadoutInitCfg{{CfgID: 3, SoulJadeConfigIDs: []int32{77}, EquippedSoulJadeConfigIDs: []int32{77, 0}, SoulJadeLevel: 7, SoulJadeEssence: 300, SoulJadeSlotCount: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := glconf.SetSoulJadeLevelCfgForTest([]*glconf.SoulJadeLevelCfg{
		{CfgID: 7, UpgradeCost: 37, RefineCost: 23, AffixCount: 1, NewAffixPermille: 9, RefineMinPermille: 9, RefineMaxPermille: 20, AllowedAttributes: []int32{4, 1}},
		{CfgID: 9, UpgradeCost: 0, RefineCost: 29, AffixCount: 2, UpgradePermille: 3, NewAffixPermille: 11, RefineMinPermille: 11, RefineMaxPermille: 30, AllowedAttributes: []int32{4, 1}},
	}); err != nil {
		t.Fatal(err)
	}
	mgr := New(100)
	if mgr == nil || mgr.GetEssence() != 300 || len(mgr.GetItems()) != 1 {
		t.Fatal("configured inventory was not applied")
	}
	view := View(mgr, "role")
	if len(view.GetSlots()) != 2 || view.GetSlots()[1].GetInstanceId() != "" {
		t.Fatal("configured empty slots missing")
	}
	item := view.GetItems()[0]
	if item.GetName() != "configured-jade" || item.GetRarity() != "configured-rarity" || item.GetIconKey() != "configured-icon" || item.GetMaxLevel() != 9 || item.GetUpgradeCost() != 37 {
		t.Fatal("configured presentation or costs missing")
	}
	if code := Upgrade(mgr, item.GetInstanceId()); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	updated := mgr.GetItems()[item.GetInstanceId()]
	if updated.GetLevel() != 9 || len(updated.GetAffixes()) != 2 || updated.GetAffixes()[0].GetPermille() != 12 || mgr.GetEssence() != 263 {
		t.Fatal("sparse configured progression was ignored")
	}
	if code := Refine(mgr, item.GetInstanceId()); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if mgr.GetEssence() != 234 {
		t.Fatal("configured reroll cost was ignored")
	}
	for _, affix := range updated.GetAffixes() {
		if affix.GetAttribute() != 4 && affix.GetAttribute() != 1 {
			t.Fatal("reroll ignored configured attribute pool")
		}
	}
	if code := Change(mgr, 3, item.GetInstanceId()); code != pb_common.ErrCode_INVALID_SLOT {
		t.Fatal("configured slot count was ignored")
	}
}

func TestMissingSoulJadeConfigurationDoesNotGrantFallbackInventory(t *testing.T) {
	loadouttest.Load(t)
	t.Cleanup(func() { loadouttest.Load(t) })
	if err := glconf.SetLoadoutInitCfgForTest(nil); err != nil {
		t.Fatal(err)
	}
	if New(100) != nil {
		t.Fatal("missing configuration used a hardcoded inventory fallback")
	}
}

func TestUpgradeCostsCapAndRefineBounds(t *testing.T) {
	loadouttest.Load(t)
	mgr := New(100)
	item := mgr.GetItems()[mgr.GetSlots()[1]]
	instanceID := item.GetID()
	before := mgr.GetEssence()
	cost := UpgradeCost(item)
	if code := Upgrade(mgr, instanceID); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if mgr.GetEssence() != before-cost || item.GetLevel() != 2 {
		t.Fatal("incorrect upgrade cost or level")
	}
	mgr.SetEssence(0)
	beforeRevision := mgr.GetRevision()
	if code := Upgrade(mgr, instanceID); code == pb_common.ErrCode_OK {
		t.Fatal("upgrade succeeded without enough materials")
	}
	if item.GetLevel() != 2 || mgr.GetRevision() != beforeRevision {
		t.Fatal("rejected upgrade partially changed state")
	}
	mgr.SetEssence(100000)
	for item.GetLevel() < maxLevel() {
		if code := Upgrade(mgr, instanceID); code != pb_common.ErrCode_OK {
			t.Fatal(code)
		}
	}
	if len(item.GetAffixes()) != 4 || UpgradeCost(item) != 0 {
		t.Fatal("incorrect max-level affix count or cost")
	}
	if code := Upgrade(mgr, instanceID); code == pb_common.ErrCode_OK {
		t.Fatal("upgrade exceeded maximum level")
	}
	before = mgr.GetEssence()
	if code := Refine(mgr, instanceID); code != pb_common.ErrCode_OK {
		t.Fatal(code)
	}
	if mgr.GetEssence() != before-RefineCost(item) {
		t.Fatal("incorrect reroll charge")
	}
	seen := map[int32]bool{}
	for _, affix := range item.GetAffixes() {
		attribute, value := affix.GetAttribute(), affix.GetPermille()
		if attribute < 1 || attribute > 4 || seen[attribute] || value < 48 || value > 56 {
			t.Fatalf("out-of-range or duplicate affix: %v", affix)
		}
		seen[attribute] = true
	}
}

func TestOnlyEquippedJadesAffectStats(t *testing.T) {
	loadouttest.Load(t)
	mgr := New(100)
	before := Bonuses(mgr).GetAttackPermille()
	item := mgr.GetItems()[mgr.GetSlots()[1]]
	if code := Change(mgr, 1, ""); code != pb_common.ErrCode_OK {
		t.Fatal("unequip failed")
	}
	if after := Bonuses(mgr).GetAttackPermille(); after != before-item.GetAffixes()[0].GetPermille() {
		t.Fatal("unequip did not change authoritative stats")
	}
}

func TestMalformedInventoryCannotPartiallyUpgradeOrGrantCurrency(t *testing.T) {
	loadouttest.Load(t)
	for _, level := range []int32{-1, 0, 1, 21} {
		mgr := New(100)
		item := mgr.GetItems()[mgr.GetSlots()[1]]
		item.SetLevel(level)
		item.SetAffixes([]*Affix{nil})
		beforeEssence, beforeRevision := mgr.GetEssence(), mgr.GetRevision()
		if code := Upgrade(mgr, item.GetID()); code == pb_common.ErrCode_OK {
			t.Fatal("malformed item was upgraded")
		}
		if code := Refine(mgr, item.GetID()); code == pb_common.ErrCode_OK {
			t.Fatal("malformed item was rerolled")
		}
		if mgr.GetEssence() != beforeEssence || mgr.GetRevision() != beforeRevision || item.GetLevel() != level {
			t.Fatal("rejected operation partially changed state")
		}
	}
}

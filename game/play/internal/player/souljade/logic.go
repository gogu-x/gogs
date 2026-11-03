package souljade

import (
	"crypto/rand"
	"sort"

	"github.com/gogu-x/gogs/game/play/internal/player/growth"
	"github.com/gogu-x/gogs/glconf"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_souljade"
)

func initConfig() *glconf.LoadoutInitCfg {
	records := glconf.GetAllLoadoutInitCfg()
	if len(records) != 1 {
		return nil
	}
	config := records[0]
	if config.GetSoulJadeSlotCount() <= 0 || config.GetSoulJadeEssence() < 0 {
		return nil
	}
	return config
}

// NormalizeSlots removes unavailable slots and duplicate references after a config change.
func NormalizeSlots(mgr *Mgr) bool {
	config := initConfig()
	if mgr == nil || config == nil {
		return false
	}
	items, slots := mgr.GetItems(), mgr.GetSlots()
	ids := make([]int, 0, len(slots))
	for slot := range slots {
		ids = append(ids, int(slot))
	}
	sort.Ints(ids)
	seen := make(map[int32]bool)
	changed := false
	for _, value := range ids {
		slot := int32(value)
		item := items[slots[slot]]
		if slot < 1 || slot > config.GetSoulJadeSlotCount() || item == nil || seen[item.GetConfigID()] {
			delete(slots, slot)
			changed = true
			continue
		}
		seen[item.GetConfigID()] = true
	}
	if changed {
		mgr.SetRevision(mgr.GetRevision() + 1)
	}
	return changed
}

func validLevel(config *glconf.SoulJadeLevelCfg) bool {
	if config == nil || config.GetCfgID() <= 0 || config.GetUpgradeCost() < 0 || config.GetRefineCost() < 0 || config.GetUpgradePermille() < 0 {
		return false
	}
	minimum, maximum := config.GetRefineMinPermille(), config.GetRefineMaxPermille()
	if minimum <= 0 || maximum < minimum || config.GetNewAffixPermille() <= 0 || config.GetNewAffixPermille() > maximum {
		return false
	}
	allowed := make(map[int32]bool)
	for _, attribute := range config.GetAllowedAttributes() {
		if attribute < 1 || attribute > 4 || allowed[attribute] {
			return false
		}
		allowed[attribute] = true
	}
	count := config.GetAffixCount()
	return count > 0 && int(count) <= len(allowed)
}

func nextLevel(current int32) *glconf.SoulJadeLevelCfg {
	var next *glconf.SoulJadeLevelCfg
	for _, config := range glconf.GetAllSoulJadeLevelCfg() {
		id := config.GetCfgID()
		if id > current && (next == nil || id < next.GetCfgID()) {
			next = config
		}
	}
	return next
}

func maxLevel() int32 {
	var maximum int32
	for _, config := range glconf.GetAllSoulJadeLevelCfg() {
		if id := config.GetCfgID(); id > maximum {
			maximum = id
		}
	}
	return maximum
}

func validItem(item *Item) bool {
	if item == nil || glconf.GetSoulJadeCfgNoErr(item.GetConfigID()) == nil {
		return false
	}
	level := glconf.GetSoulJadeLevelCfgNoErr(item.GetLevel())
	if !validLevel(level) {
		return false
	}
	affixes := item.GetAffixes()
	if len(affixes) != int(level.GetAffixCount()) {
		return false
	}
	allowed := make(map[int32]bool)
	for _, attribute := range level.GetAllowedAttributes() {
		allowed[attribute] = true
	}
	seen := make(map[int32]bool)
	for _, affix := range affixes {
		attribute, value := affix.GetAttribute(), affix.GetPermille()
		if affix == nil || seen[attribute] {
			return false
		}
		if effect := glconf.GetSoulJadeEffectAffixCfgNoErr(attribute); effect != nil {
			if !glconf.ValidSoulJadeEffectAffixCfg(effect) || value != 0 {
				return false
			}
		} else if !allowed[attribute] || value <= 0 || value > level.GetRefineMaxPermille() {
			return false
		}
		seen[attribute] = true
	}
	return true
}

func fillAffixes(item *Item, level *glconf.SoulJadeLevelCfg) bool {
	used := make(map[int32]bool)
	for _, affix := range item.GetAffixes() {
		used[affix.GetAttribute()] = true
	}
	for _, attribute := range level.GetAllowedAttributes() {
		if len(item.GetAffixes()) >= int(level.GetAffixCount()) {
			break
		}
		if !used[attribute] {
			item.AddAffix(&Affix{Attribute: attribute, Permille: level.GetNewAffixPermille()})
			used[attribute] = true
		}
	}
	return len(item.GetAffixes()) == int(level.GetAffixCount())
}

// Change validates ownership, configured slots and definition uniqueness.
func Change(mgr *Mgr, slot int32, instanceID string) pb_common.ErrCode {
	if mgr == nil {
		return pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	config := initConfig()
	if config == nil {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	if slot < 1 || slot > config.GetSoulJadeSlotCount() {
		return pb_common.ErrCode_INVALID_SLOT
	}
	items, slots := mgr.GetItems(), mgr.GetSlots()
	if instanceID != "" {
		item := items[instanceID]
		if item == nil {
			return pb_common.ErrCode_ITEM_NOT_OWNED
		}
		if !validItem(item) {
			return pb_common.ErrCode_INVALID_ITEM_STATE
		}
		for equippedSlot, equippedID := range slots {
			if equippedSlot == slot {
				continue
			}
			if equipped := items[equippedID]; equipped != nil && equipped.GetConfigID() == item.GetConfigID() {
				return pb_common.ErrCode_DUPLICATE_JADE
			}
		}
	}
	if slots == nil {
		slots = make(map[int32]string)
		mgr.SetSlots(slots)
	}
	if instanceID == "" {
		delete(slots, slot)
	} else {
		slots[slot] = instanceID
	}
	mgr.SetRevision(mgr.GetRevision() + 1)
	return pb_common.ErrCode_OK
}

// UpgradeCost returns the configured cost for leaving the current level.
func UpgradeCost(item *Item) int64 {
	level := glconf.GetSoulJadeLevelCfgNoErr(item.GetLevel())
	if !validLevel(level) || nextLevel(item.GetLevel()) == nil {
		return 0
	}
	return level.GetUpgradeCost()
}

// RefineCost returns the current configured reroll cost.
func RefineCost(item *Item) int64 {
	if level := glconf.GetSoulJadeLevelCfgNoErr(item.GetLevel()); validLevel(level) {
		return level.GetRefineCost()
	}
	return 0
}

// Upgrade stages all changes before spending materials or updating inventory.
func Upgrade(mgr *Mgr, instanceID string) pb_common.ErrCode {
	if mgr == nil {
		return pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	item := mgr.GetItems()[instanceID]
	if item == nil {
		return pb_common.ErrCode_ITEM_NOT_OWNED
	}
	if !validItem(item) {
		return pb_common.ErrCode_INVALID_ITEM_STATE
	}
	next := nextLevel(item.GetLevel())
	if next == nil {
		return pb_common.ErrCode_MAX_LEVEL_REACHED
	}
	if !validLevel(next) {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	cost := UpgradeCost(item)
	if mgr.GetEssence() < cost {
		return pb_common.ErrCode_INSUFFICIENT_ESSENCE
	}
	updated := item.DeepCopy()
	updated.SetLevel(next.GetCfgID())
	for _, affix := range updated.GetAffixes() {
		if glconf.GetSoulJadeEffectAffixCfgNoErr(affix.GetAttribute()) != nil {
			continue
		}
		value := int64(affix.GetPermille()) + int64(next.GetUpgradePermille())
		if value > int64(next.GetRefineMaxPermille()) {
			return pb_common.ErrCode_CONFIG_UNAVAILABLE
		}
		affix.SetPermille(int32(value))
	}
	if !fillAffixes(updated, next) || !validItem(updated) {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	item.SetLevel(updated.GetLevel())
	item.SetAffixes(updated.GetAffixes())
	mgr.SetEssence(mgr.GetEssence() - cost)
	mgr.SetRevision(mgr.GetRevision() + 1)
	return pb_common.ErrCode_OK
}

// Refine selects distinct configured attributes and values on the server.
func Refine(mgr *Mgr, instanceID string) pb_common.ErrCode {
	if mgr == nil {
		return pb_common.ErrCode_LOADOUT_UNAVAILABLE
	}
	item := mgr.GetItems()[instanceID]
	if item == nil {
		return pb_common.ErrCode_ITEM_NOT_OWNED
	}
	if !validItem(item) {
		return pb_common.ErrCode_INVALID_ITEM_STATE
	}
	level := glconf.GetSoulJadeLevelCfgNoErr(item.GetLevel())
	cost := level.GetRefineCost()
	if mgr.GetEssence() < cost {
		return pb_common.ErrCode_INSUFFICIENT_ESSENCE
	}
	pool, ok := refinePool(level)
	if !ok {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	affixes, err := rollRefineAffixes(pool, level, rand.Reader)
	if err != nil {
		return pb_common.ErrCode_AFFIX_GENERATION_FAILED
	}
	updated := item.DeepCopy()
	updated.SetAffixes(affixes)
	if !validItem(updated) {
		return pb_common.ErrCode_CONFIG_UNAVAILABLE
	}
	item.SetAffixes(affixes)
	mgr.SetEssence(mgr.GetEssence() - cost)
	mgr.SetRevision(mgr.GetRevision() + 1)
	return pb_common.ErrCode_OK
}

// Bonuses includes only configured, owned, equipped and distinct definitions.
func Bonuses(mgr *Mgr) *pb_common.GrowthBonuses {
	result := &pb_common.GrowthBonuses{}
	config := initConfig()
	if mgr == nil || config == nil {
		return result
	}
	items, slots := mgr.GetItems(), mgr.GetSlots()
	seen := make(map[int32]bool)
	for slot := int32(1); slot <= config.GetSoulJadeSlotCount(); slot++ {
		item := items[slots[slot]]
		if !validItem(item) {
			continue
		}
		configID := item.GetConfigID()
		if seen[configID] {
			continue
		}
		seen[configID] = true
		for _, affix := range item.GetAffixes() {
			if glconf.GetSoulJadeEffectAffixCfgNoErr(affix.GetAttribute()) == nil {
				growth.Add(result, growth.Attribute(affix.GetAttribute(), affix.GetPermille()))
			}
		}
	}
	return result
}

// View publishes configured names, icons, costs, slots and battle modifiers.
func View(mgr *Mgr, roleID string) *pb_souljade.RoleSoulJadeData {
	config := initConfig()
	if mgr == nil || config == nil {
		return nil
	}
	items := mgr.GetItems()
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	viewItems := make([]*pb_souljade.SoulJadeItem, 0, len(items))
	for _, key := range keys {
		item := items[key]
		if !validItem(item) {
			continue
		}
		definition := glconf.GetSoulJadeCfgNoErr(item.GetConfigID())
		affixes := make([]*pb_souljade.SoulJadeAffix, 0, len(item.GetAffixes()))
		for _, affix := range item.GetAffixes() {
			affixes = append(affixes, &pb_souljade.SoulJadeAffix{Attribute: affix.GetAttribute(), Permille: affix.GetPermille()})
		}
		viewItems = append(viewItems, &pb_souljade.SoulJadeItem{InstanceId: key, ConfigId: item.GetConfigID(), Name: definition.GetName(), Rarity: definition.GetRarity(), IconKey: definition.GetIconKey(), Level: item.GetLevel(), MaxLevel: maxLevel(), Affixes: affixes, UpgradeCost: UpgradeCost(item), RefineCost: RefineCost(item)})
	}
	slots := mgr.GetSlots()
	viewSlots := make([]*pb_souljade.SoulJadeSlot, 0, config.GetSoulJadeSlotCount())
	for slot := int32(1); slot <= config.GetSoulJadeSlotCount(); slot++ {
		viewSlots = append(viewSlots, &pb_souljade.SoulJadeSlot{Slot: slot, InstanceId: slots[slot]})
	}
	return &pb_souljade.RoleSoulJadeData{Revision: mgr.GetRevision(), Essence: mgr.GetEssence(), Items: viewItems, Slots: viewSlots, Bonuses: Bonuses(mgr), RoleId: roleID}
}

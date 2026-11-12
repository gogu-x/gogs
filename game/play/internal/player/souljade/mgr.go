package souljade

import (
	"fmt"
	"github.com/gogu-x/gogs/glconf"
)

// Affix stores a server-generated stat modifier.
// +deepcopy-gen=true
//
//go:generate accessor-gen
type Affix struct {
	Attribute int32 `bson:"attribute"` // Supported battle attribute identifier.
	Permille  int32 `bson:"permille"`  // Additive growth in permille.
}

// Item stores one owned soul jade instance.
// +deepcopy-gen=true
//
//go:generate accessor-gen
type Item struct {
	ID       string   `bson:"id"`        // Unique instance identifier.
	ConfigID int32    `bson:"config_id"` // Definition identifier.
	Level    int32    `bson:"level"`     // Configured progression level.
	Affixes  []*Affix `bson:"affixes"`   // Active rolled modifiers.
}

// AddAffix adds a generated modifier.
func (item *Item) AddAffix(affix *Affix) { item.Affixes = append(item.GetAffixes(), affix) }

// Mgr stores owned instances, materials and generic slots.
// +deepcopy-gen=true
//
//go:generate accessor-gen
type Mgr struct {
	Revision uint64           `bson:"revision"` // Mutation revision used for optimistic concurrency.
	Essence  int64            `bson:"essence"`  // Available upgrade and reroll materials.
	Items    map[string]*Item `bson:"items"`    // Owned instances.
	Slots    map[int32]string `bson:"slots"`    // Slot-to-instance references.
}

// New initializes inventory exclusively from loaded configuration.
func New(uid uint64) *Mgr {
	config := initConfig()
	if config == nil {
		return nil
	}
	level := glconf.GetSoulJadeLevelCfgNoErr(config.GetSoulJadeLevel())
	if !validLevel(level) {
		return nil
	}
	items := make(map[string]*Item)
	instances := make(map[int32]string)
	for _, configID := range config.GetSoulJadeConfigIDs() {
		definition := glconf.GetSoulJadeCfgNoErr(configID)
		if definition == nil || instances[configID] != "" {
			return nil
		}
		id := fmt.Sprintf("%d-jade-%02d", uid, configID)
		item := &Item{ID: id, ConfigID: configID, Level: level.GetCfgID(), Affixes: []*Affix{{Attribute: definition.GetInitialAttribute(), Permille: definition.GetInitialPermille()}}}
		if !fillAffixes(item, level) || !validItem(item) {
			return nil
		}
		items[id], instances[configID] = item, id
	}
	slots := make(map[int32]string)
	equipped := config.GetEquippedSoulJadeConfigIDs()
	if len(equipped) > int(config.GetSoulJadeSlotCount()) {
		return nil
	}
	seen := make(map[int32]bool)
	for index, configID := range equipped {
		if configID == 0 {
			continue
		}
		if instances[configID] == "" || seen[configID] {
			return nil
		}
		seen[configID] = true
		slots[int32(index)+1] = instances[configID]
	}
	return &Mgr{Revision: 1, Essence: config.GetSoulJadeEssence(), Items: items, Slots: slots}
}

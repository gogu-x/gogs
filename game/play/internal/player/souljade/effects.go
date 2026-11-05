package souljade

import (
	"github.com/gogu-x/gogs/glconf"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// EffectAffixIDs 返回已装备魂玉的有效效果词条，同类型效果按槽位顺序取首个。
func EffectAffixIDs(mgr *Mgr) []int32 {
	config := initConfig()
	if mgr == nil || config == nil {
		return nil
	}
	items, slots := mgr.GetItems(), mgr.GetSlots()
	seenItems := make(map[int32]bool)
	seenEffects := make(map[pb.EffectType]bool)
	var ids []int32
	for slot := int32(1); slot <= config.GetSoulJadeSlotCount(); slot++ {
		item := items[slots[slot]]
		if !validItem(item) || seenItems[item.GetConfigID()] {
			continue
		}
		seenItems[item.GetConfigID()] = true
		for _, affix := range item.GetAffixes() {
			effect := glconf.GetSoulJadeEffectAffixCfgNoErr(affix.GetAttribute())
			if !glconf.ValidSoulJadeEffectAffixCfg(effect) || seenEffects[effect.GetEffectType()] {
				continue
			}
			seenEffects[effect.GetEffectType()] = true
			ids = append(ids, effect.GetCfgID())
		}
	}
	return ids
}

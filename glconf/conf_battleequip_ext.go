package glconf

import (
	"sort"
	"sync/atomic"

	"github.com/gogu-x/gogs/pb/cspb/pb_common"
)

type equipmentSlots []*EquipmentSlotCfg
type equipmentAttributes map[*BattleEquipCfg][4]int64
type statusAttributes map[*BattleStatusCfg][4]int64

var orderedEquipmentSlots atomic.Pointer[equipmentSlots]
var battleEquipAttributes atomic.Pointer[equipmentAttributes]
var battleStatusAttributes atomic.Pointer[statusAttributes]

func loadEquipmentSlots(table *CsvConf) {
	slots := make(equipmentSlots, 0, table.NumRecord())
	for index := range table.NumRecord() {
		slots = append(slots, table.Record(index).(*EquipmentSlotCfg))
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].GetCfgID() < slots[j].GetCfgID() })
	orderedEquipmentSlots.Store(&slots)
}

func loadBattleEquipAttributes(table *CsvConf) {
	result := make(equipmentAttributes, table.NumRecord())
	for index := range table.NumRecord() {
		row := table.Record(index).(*BattleEquipCfg)
		result[row] = sumBattleAttributes(row.GetModifier())
	}
	battleEquipAttributes.Store(&result)
}

func loadBattleStatusAttributes(table *CsvConf) {
	result := make(statusAttributes, table.NumRecord())
	for index := range table.NumRecord() {
		row := table.Record(index).(*BattleStatusCfg)
		result[row] = sumBattleAttributes(row.configModifiers())
	}
	battleStatusAttributes.Store(&result)
}

func (row *BattleStatusCfg) configModifiers() []*pb_common.TypIDVal {
	if row == nil {
		return nil
	}
	return row.Modifier
}

// GetOrderedEquipmentSlots 返回按编号排序的装备槽位，列表供只读使用。
func GetOrderedEquipmentSlots() []*EquipmentSlotCfg {
	if slots := orderedEquipmentSlots.Load(); slots != nil {
		return *slots
	}
	return nil
}

// GetBattleEquipAttributes 返回装备加成，顺序为生命、攻击、防御、速度。
func GetBattleEquipAttributes(row *BattleEquipCfg) [4]int64 {
	if attributes := battleEquipAttributes.Load(); attributes != nil {
		if values, ok := (*attributes)[row]; ok {
			return values
		}
	}
	return sumBattleAttributes(row.GetModifier())
}

// GetBattleStatusAttributes 返回状态加成，顺序为生命、攻击、防御、速度。
func GetBattleStatusAttributes(row *BattleStatusCfg) [4]int64 {
	if attributes := battleStatusAttributes.Load(); attributes != nil {
		if values, ok := (*attributes)[row]; ok {
			return values
		}
	}
	return sumBattleAttributes(row.configModifiers())
}

func sumBattleAttributes(modifiers []*pb_common.TypIDVal) [4]int64 {
	var totals [4]int64
	for _, modifier := range modifiers {
		if modifier.GetTyp() != "battle_attribute" {
			continue
		}
		if index := int(modifier.GetId()) - 1; index >= 0 && index < len(totals) {
			totals[index] += modifier.GetVal()
		}
	}
	return totals
}

package glconf

import (
	"fmt"
	"sync/atomic"
)

const (
	// BattleRoleCfgKey 是角色战斗配置表名称。
	BattleRoleCfgKey = "BattleRoleCfg"
	// BattleSkillCfgKey 是战斗技能配置表名称。
	BattleSkillCfgKey = "BattleSkillCfg"
	// BattleEquipCfgKey 是战斗装备配置表名称。
	BattleEquipCfgKey = "BattleEquipCfg"
	// BattleMonsterCfgKey 是战斗怪物配置表名称。
	BattleMonsterCfgKey = "BattleMonsterCfg"
	// BattleMonsterGroupCfgKey 是战斗怪物组配置表名称。
	BattleMonsterGroupCfgKey = "BattleMonsterGroupCfg"
	// BattleRuleCfgKey 是战斗规则配置表名称。
	BattleRuleCfgKey = "BattleRuleCfg"
	// BattleStatusCfgKey 是战斗状态配置表名称。
	BattleStatusCfgKey = "BattleStatusCfg"
)

var (
	csvBattleRoleCfg         atomic.Value
	csvBattleSkillCfg        atomic.Value
	csvBattleEquipCfg        atomic.Value
	csvBattleMonsterCfg      atomic.Value
	csvBattleMonsterGroupCfg atomic.Value
	csvBattleRuleCfg         atomic.Value
	csvBattleStatusCfg       atomic.Value
)

// BattleAttributeModifierCfg 表示装备或状态提供的基础属性修正。
type BattleAttributeModifierCfg struct {
	MaxHP   int64 `bson:"max_hp"`  // 最大生命修正
	Attack  int64 `bson:"attack"`  // 攻击修正
	Defense int64 `bson:"defense"` // 防御修正
	Speed   int64 `bson:"speed"`   // 速度修正
}

// BattleEffectCfg 表示技能中的一个顺序效果。
type BattleEffectCfg struct {
	Kind                int32 `bson:"kind"`                 // 效果类型
	CoefficientPermille int64 `bson:"coefficient_permille"` // 伤害或治疗倍率
	Flat                int64 `bson:"flat"`                 // 固定数值
	StatusID            int32 `bson:"status_id"`            // 状态配置 ID
}

// BattleStatusCfg 表示可施加或初始生效的状态配置。
type BattleStatusCfg struct {
	CfgID         int32                      `bson:"id"`             // 状态配置 ID
	Kind          int32                      `bson:"kind"`           // 状态类型
	DurationTurns int32                      `bson:"duration_turns"` // 持续行动次数
	Potency       int64                      `bson:"potency"`        // DOT 或 HOT 数值
	Modifier      BattleAttributeModifierCfg `bson:"modifier"`       // 属性修正
}

// BattleRoleCfg 表示角色的静态战斗配置。
type BattleRoleCfg struct {
	CfgID            int32   `bson:"id"`                 // 角色配置 ID
	MaxHP            int64   `bson:"max_hp"`             // 基础最大生命
	Attack           int64   `bson:"attack"`             // 基础攻击
	Defense          int64   `bson:"defense"`            // 基础防御
	Speed            int64   `bson:"speed"`              // 基础速度
	DefaultPosition  int32   `bson:"default_position"`   // 默认站位
	BasicSkillID     int32   `bson:"basic_skill_id"`     // 普攻技能 ID
	ActiveSkillIDs   []int32 `bson:"active_skill_ids"`   // 主动技能 ID
	InitialStatusIDs []int32 `bson:"initial_status_ids"` // 初始状态 ID
}

// BattleSkillCfg 表示技能的静态战斗配置。
type BattleSkillCfg struct {
	CfgID      int32             `bson:"id"`          // 技能配置 ID
	Cooldown   int32             `bson:"cooldown"`    // 冷却行动次数
	TargetRule int32             `bson:"target_rule"` // 目标规则
	Effects    []BattleEffectCfg `bson:"effects"`     // 顺序效果
}

// BattleEquipCfg 表示装备的静态战斗配置。
type BattleEquipCfg struct {
	CfgID    int32                      `bson:"id"`       // 装备配置 ID
	Modifier BattleAttributeModifierCfg `bson:"modifier"` // 属性修正
}

// BattleMonsterCfg 表示怪物的静态战斗配置。
type BattleMonsterCfg struct {
	CfgID            int32   `bson:"id"`                 // 怪物配置 ID
	MaxHP            int64   `bson:"max_hp"`             // 基础最大生命
	Attack           int64   `bson:"attack"`             // 基础攻击
	Defense          int64   `bson:"defense"`            // 基础防御
	Speed            int64   `bson:"speed"`              // 基础速度
	BasicSkillID     int32   `bson:"basic_skill_id"`     // 普攻技能 ID
	ActiveSkillIDs   []int32 `bson:"active_skill_ids"`   // 主动技能 ID
	InitialStatusIDs []int32 `bson:"initial_status_ids"` // 初始状态 ID
}

// BattleMonsterGroupMemberCfg 表示怪物组中的一个怪物和其配置站位。
type BattleMonsterGroupMemberCfg struct {
	MonsterCfgID int32 `bson:"monster_cfg_id"` // 怪物配置 ID
	Position     int32 `bson:"position"`       // 配置站位
}

// BattleMonsterGroupCfg 表示 PvE 敌方怪物组配置。
type BattleMonsterGroupCfg struct {
	CfgID   int32                         `bson:"id"`      // 怪物组配置 ID
	Members []BattleMonsterGroupMemberCfg `bson:"members"` // 怪物成员
}

// BattleRuleCfg 表示一种战斗类型的全局规则。
type BattleRuleCfg struct {
	BattleType             int32 `bson:"id"`                       // 战斗类型
	ATBThreshold           int64 `bson:"atb_threshold"`            // 行动条阈值
	MaxActions             int32 `bson:"max_actions"`              // 最大行动次数
	DamageVariancePermille int64 `bson:"damage_variance_permille"` // 伤害浮动
	CritChancePermille     int64 `bson:"crit_chance_permille"`     // 暴击概率
	CritMultiplierPermille int64 `bson:"crit_multiplier_permille"` // 暴击倍率
}

func loadBattleRoleCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleRoleCfg, dbURI, dbName, confName, &BattleRoleCfg{})
}

func loadBattleSkillCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleSkillCfg, dbURI, dbName, confName, &BattleSkillCfg{})
}

func loadBattleEquipCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleEquipCfg, dbURI, dbName, confName, &BattleEquipCfg{})
}

func loadBattleMonsterCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleMonsterCfg, dbURI, dbName, confName, &BattleMonsterCfg{})
}

func loadBattleMonsterGroupCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleMonsterGroupCfg, dbURI, dbName, confName, &BattleMonsterGroupCfg{})
}

func loadBattleRuleCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleRuleCfg, dbURI, dbName, confName, &BattleRuleCfg{})
}

func loadBattleStatusCfg(dbURI, dbName, confName string) (*CsvConf, error) {
	return loadBattleConf(&csvBattleStatusCfg, dbURI, dbName, confName, &BattleStatusCfg{})
}

func loadBattleConf(
	target *atomic.Value,
	dbURI,
	dbName,
	confName string,
	prototype interface{},
) (*CsvConf, error) {
	table, err := readCsv(dbURI, dbName, confName, prototype)
	if err != nil {
		return nil, err
	}
	target.Store(table)
	return table, nil
}

// GetBattleRoleCfg 按配置 ID 查询角色战斗配置。
func GetBattleRoleCfg(id int32) *BattleRoleCfg {
	return getBattleCfg[BattleRoleCfg](&csvBattleRoleCfg, id)
}

// GetBattleSkillCfg 按配置 ID 查询技能战斗配置。
func GetBattleSkillCfg(id int32) *BattleSkillCfg {
	return getBattleCfg[BattleSkillCfg](&csvBattleSkillCfg, id)
}

// GetBattleEquipCfg 按配置 ID 查询装备战斗配置。
func GetBattleEquipCfg(id int32) *BattleEquipCfg {
	return getBattleCfg[BattleEquipCfg](&csvBattleEquipCfg, id)
}

// GetBattleMonsterCfg 按配置 ID 查询怪物战斗配置。
func GetBattleMonsterCfg(id int32) *BattleMonsterCfg {
	return getBattleCfg[BattleMonsterCfg](&csvBattleMonsterCfg, id)
}

// GetBattleMonsterGroupCfg 按配置 ID 查询怪物组战斗配置。
func GetBattleMonsterGroupCfg(id int32) *BattleMonsterGroupCfg {
	return getBattleCfg[BattleMonsterGroupCfg](&csvBattleMonsterGroupCfg, id)
}

// GetBattleRuleCfg 按战斗类型查询战斗规则配置。
func GetBattleRuleCfg(battleType int32) *BattleRuleCfg {
	return getBattleCfg[BattleRuleCfg](&csvBattleRuleCfg, battleType)
}

// GetBattleStatusCfg 按配置 ID 查询状态战斗配置。
func GetBattleStatusCfg(id int32) *BattleStatusCfg {
	return getBattleCfg[BattleStatusCfg](&csvBattleStatusCfg, id)
}

func getBattleCfg[T any](source *atomic.Value, id int32) *T {
	if table := asCsv(*source); table != nil {
		if cfg := table.Get(id); cfg != nil {
			return cfg.(*T)
		}
	}
	return nil
}

// ValidateBattleMonsterGroupCfg 校验怪物组成员引用和配置站位。
func ValidateBattleMonsterGroupCfg(group *BattleMonsterGroupCfg) error {
	if group == nil {
		return fmt.Errorf("battle monster group config is required")
	}
	members := group.Members
	if len(members) == 0 {
		return fmt.Errorf("battle monster group config id=%d has no members", group.CfgID)
	}
	positions := make(map[int32]struct{}, len(members))
	for _, member := range members {
		if member.MonsterCfgID == 0 {
			return fmt.Errorf("battle monster group config id=%d has empty monster config id", group.CfgID)
		}
		if GetBattleMonsterCfg(member.MonsterCfgID) == nil {
			return fmt.Errorf("battle monster group config id=%d references missing monster config id=%d", group.CfgID, member.MonsterCfgID)
		}
		if _, exists := positions[member.Position]; exists {
			return fmt.Errorf("battle monster group config id=%d has duplicate position=%d", group.CfgID, member.Position)
		}
		positions[member.Position] = struct{}{}
	}
	return nil
}

// SetBattleConfigsForTest 安装测试专用的战斗配置数据。
func SetBattleConfigsForTest(
	roles []*BattleRoleCfg,
	skills []*BattleSkillCfg,
	equips []*BattleEquipCfg,
	monsters []*BattleMonsterCfg,
	groups []*BattleMonsterGroupCfg,
	rules []*BattleRuleCfg,
) error {
	if err := setBattleCfgTable(&csvBattleRoleCfg, roles); err != nil {
		return err
	}
	if err := setBattleCfgTable(&csvBattleSkillCfg, skills); err != nil {
		return err
	}
	if err := setBattleCfgTable(&csvBattleEquipCfg, equips); err != nil {
		return err
	}
	if err := setBattleCfgTable(&csvBattleMonsterCfg, monsters); err != nil {
		return err
	}
	if err := setBattleCfgTable(&csvBattleMonsterGroupCfg, groups); err != nil {
		return err
	}
	return setBattleCfgTable(&csvBattleRuleCfg, rules)
}

func setBattleCfgTable[T any](target *atomic.Value, records []*T) error {
	values := make([]interface{}, 0, len(records))
	for _, record := range records {
		values = append(values, record)
	}
	table, err := newCsv(new(T))
	if err != nil {
		return err
	}
	if err := table.replaceRecords("test", values); err != nil {
		return err
	}
	target.Store(table)
	return nil
}

// SetBattleStatusConfigsForTest 安装测试专用的状态配置数据。
func SetBattleStatusConfigsForTest(statuses []*BattleStatusCfg) error {
	return setBattleCfgTable(&csvBattleStatusCfg, statuses)
}

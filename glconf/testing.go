package glconf

// installRecordsForTest 用内存记录替换单张配置表，供不连接 MongoDB 的单元测试使用。
func installRecordsForTest(prototype interface{}, records []interface{}, store func(*CsvConf)) error {
	table, err := newCsv(prototype)
	if err != nil {
		return err
	}
	if err := table.replaceRecords("test", records); err != nil {
		return err
	}
	store(table)
	return nil
}

func interfacesOf[T any](values []*T) []interface{} {
	records := make([]interface{}, 0, len(values))
	for _, value := range values {
		records = append(records, value)
	}
	return records
}

// SetBattleConfigsForTest 安装战斗工厂测试所需的核心配置。
func SetBattleConfigsForTest(
	roles []*BattleRoleCfg,
	skills []*BattleSkillCfg,
	equips []*BattleEquipCfg,
	monsters []*BattleMonsterCfg,
	groups []*BattleMonsterGroupCfg,
	rules []*BattleRuleCfg,
) error {
	installers := []func() error{
		func() error {
			return installRecordsForTest(&BattleRoleCfg{}, interfacesOf(roles), func(table *CsvConf) { csvBattleRoleCfg.Store(table) })
		},
		func() error {
			return installRecordsForTest(&BattleSkillCfg{}, interfacesOf(skills), func(table *CsvConf) { csvBattleSkillCfg.Store(table) })
		},
		func() error {
			return installRecordsForTest(&BattleEquipCfg{}, interfacesOf(equips), func(table *CsvConf) { csvBattleEquipCfg.Store(table) })
		},
		func() error {
			return installRecordsForTest(&BattleMonsterCfg{}, interfacesOf(monsters), func(table *CsvConf) { csvBattleMonsterCfg.Store(table) })
		},
		func() error {
			return installRecordsForTest(&BattleMonsterGroupCfg{}, interfacesOf(groups), func(table *CsvConf) { csvBattleMonsterGroupCfg.Store(table) })
		},
		func() error {
			return installRecordsForTest(&BattleRuleCfg{}, interfacesOf(rules), func(table *CsvConf) { csvBattleRuleCfg.Store(table) })
		},
	}
	for _, install := range installers {
		if err := install(); err != nil {
			return err
		}
	}
	return nil
}

// SetBattleStatusConfigsForTest 安装状态配置。
func SetBattleStatusConfigsForTest(statuses []*BattleStatusCfg) error {
	return installRecordsForTest(&BattleStatusCfg{}, interfacesOf(statuses), func(table *CsvConf) {
		csvBattleStatusCfg.Store(table)
	})
}

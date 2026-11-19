package glconf

import "sync/atomic"

type initialRoleProfiles map[int32]*PlayerInitRoleCfg

var validInitialRoles atomic.Pointer[initialRoleProfiles]

func loadInitialRoleProfiles(profiles *CsvConf) {
	roles, _ := csvBattleRoleCfg.Load().(*CsvConf)
	rebuildInitialRoleProfiles(profiles, roles)
}

func loadInitialBattleRoles(roles *CsvConf) {
	profiles, _ := csvPlayerInitRoleCfg.Load().(*CsvConf)
	rebuildInitialRoleProfiles(profiles, roles)
}

func rebuildInitialRoleProfiles(profiles, roles *CsvConf) {
	valid := make(initialRoleProfiles)
	for index := range profiles.NumRecord() {
		row := profiles.Record(index).(*PlayerInitRoleCfg)
		roleID := row.GetRoleConfigID()
		if roleID > 0 && row.GetLevel() > 0 && roles.Get(roleID) != nil {
			valid[row.GetCfgID()] = row
		}
	}
	validInitialRoles.Store(&valid)
}

// GetValidPlayerInitRole 返回具有有效角色引用的初始角色配置。
func GetValidPlayerInitRole(id int32) *PlayerInitRoleCfg {
	if profiles := validInitialRoles.Load(); profiles != nil {
		return (*profiles)[id]
	}
	return nil
}

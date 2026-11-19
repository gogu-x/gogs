package glconf

import "sync"

var registerConfigCallbacksOnce = sync.OnceFunc(registerConfigCallbacks)

// registerConfigCallbacks 在配置表初始化后、数据加载前注册内置回调。
func registerConfigCallbacks() {
	RegisterAfterLoad(ActvRewardCfgKey, onShopRewardConfigLoaded)
	RegisterAfterLoad(BattleEquipCfgKey, onShopEquipmentConfigLoaded)
	RegisterAfterLoad(ActvShopGoodsCfgKey, rebuildShopGoodsGroups)
	RegisterAfterLoad(TalentRuleCfgKey, loadTalentRule)
	RegisterAfterLoad(TalentNodeCfgKey, loadTalentNodes)
	RegisterAfterLoad(TalentBranchCfgKey, loadTalentBranches)
	RegisterAfterLoad(LoadoutInitCfgKey, loadSoulJadeInit)
	RegisterAfterLoad(SoulJadeLevelCfgKey, loadSoulJadeLevels)
	RegisterAfterLoad(SoulJadeEffectAffixCfgKey, loadSoulJadeEffects)
	RegisterAfterLoad(EquipmentSlotCfgKey, loadEquipmentSlots)
	RegisterAfterLoad(BattleEquipCfgKey, loadBattleEquipAttributes)
	RegisterAfterLoad(BattleStatusCfgKey, loadBattleStatusAttributes)
	RegisterAfterLoad(ActvOnlineCfgKey, loadActvTiming)
	RegisterAfterLoad(BattleMonsterGroupCfgKey, loadBattleMonsterGroups)
	RegisterAfterLoad(PlayerInitRoleCfgKey, loadInitialRoleProfiles)
	RegisterAfterLoad(BattleRoleCfgKey, loadInitialBattleRoles)

	RegisterAfterLoad(ActvTaskLotteryCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(ActvLotteryPoolCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(ActvTaskCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(ActvLotteryRewardCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(ActvRewardCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(BattleEquipCfgKey, rebuildTaskLotterySettings)
	RegisterAfterLoad(ActvShopGoodsCfgKey, rebuildTaskLotterySettings)
}

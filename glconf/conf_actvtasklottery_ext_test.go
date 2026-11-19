package glconf

import (
	"sync/atomic"
	"testing"
)

func TestTaskLotterySettingsFollowConfigLoads(t *testing.T) {
	tables := []*atomic.Value{
		&csvActvOnlineCfg, &csvActvTaskLotteryCfg, &csvActvLotteryPoolCfg,
		&csvActvTaskCfg, &csvActvLotteryRewardCfg, &csvActvRewardCfg,
		&csvBattleEquipCfg, &csvActvShopGoodsCfg,
	}
	previous := make([]*CsvConf, len(tables))
	for index, table := range tables {
		previous[index], _ = table.Load().(*CsvConf)
		table.Store((*CsvConf)(nil))
	}
	previousSettings := taskLotterySettings.Swap(nil)
	previousTiming := actvTimingCatalog.Load()
	previousRewards, _ := shopRewardGroups.Load().(shopRewardIndex)
	previousGoods := shopGoodsGroups.Load()
	previousEquipment := battleEquipAttributes.Load()
	t.Cleanup(func() {
		for index, table := range tables {
			table.Store(previous[index])
		}
		taskLotterySettings.Store(previousSettings)
		actvTimingCatalog.Store(previousTiming)
		shopRewardGroups.Store(previousRewards)
		shopGoodsGroups.Store(previousGoods)
		battleEquipAttributes.Store(previousEquipment)
	})

	installers := []func() error{
		func() error {
			return SetActvOnlineCfgForTest([]*ActvOnlineCfg{{ID: 10, ContentID: 11}})
		},
		func() error {
			return SetActvTaskLotteryCfgForTest([]*ActvTaskLotteryCfg{{CfgID: 11, TaskGroupID: 12, LotteryPoolID: 13, UnclaimedDailyPolicy: 1}})
		},
		func() error {
			return SetActvLotteryPoolCfgForTest([]*ActvLotteryPoolCfg{{CfgID: 13, CostItemID: 1, CostCount: 2, DrawMode: 1, DrawCounts: []int32{1, 5}, RewardGroupID: 14}})
		},
		func() error {
			return SetActvTaskCfgForTest([]*ActvTaskCfg{{CfgID: 20, GroupID: 12, GoalType: 1, Target: 1, RewardGroupID: 16}})
		},
		func() error {
			return SetActvLotteryRewardCfgForTest([]*ActvLotteryRewardCfg{{CfgID: 21, GroupID: 14, RewardGroupID: 16, Weight: 7}})
		},
		func() error {
			return SetActvRewardCfgForTest([]*ActvRewardCfg{{CfgID: 22, GroupID: 16, RewardType: 2, RewardID: 23, Count: 1}})
		},
		func() error { return SetActvShopGoodsCfgForTest(nil) },
		func() error { return SetBattleEquipCfgForTest([]*BattleEquipCfg{{CfgID: 23}}) },
	}
	for _, install := range installers {
		if err := install(); err != nil {
			t.Fatal(err)
		}
	}

	settings := GetTaskLotterySettings(10)
	if settings == nil || len(settings.GetTasks()) != 1 || len(settings.GetRewards(16)) != 1 ||
		settings.GetWeight() != 7 || !settings.AllowsDrawCount(5) || settings.AllowsDrawCount(2) {
		t.Fatalf("unexpected lottery settings: %v", settings)
	}
	if err := SetBattleEquipCfgForTest(nil); err != nil {
		t.Fatal(err)
	}
	if GetTaskLotterySettings(10) != nil {
		t.Fatal("retained lottery settings with a removed equipment reference")
	}
}

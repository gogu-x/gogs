package glconf

import (
	"math"
	"sync/atomic"

	"github.com/gogu-x/gogs/ipb"
)

// TaskLotterySettings 保存加载时校验好的任务和抽奖规则，返回的条目供只读使用。
type TaskLotterySettings struct {
	pool       *ActvLotteryPoolCfg
	tasks      []*ActvTaskCfg
	rewards    map[int32][]*ipb.ActvAssetReward
	draws      []*ActvLotteryRewardCfg
	drawCounts map[int32]bool
	weight     int64
}

// GetPool 返回抽奖池配置。
func (s *TaskLotterySettings) GetPool() *ActvLotteryPoolCfg { return s.pool }

// GetTasks 返回任务组。
func (s *TaskLotterySettings) GetTasks() []*ActvTaskCfg { return s.tasks }

// GetRewards 返回指定奖励组。
func (s *TaskLotterySettings) GetRewards(group int32) []*ipb.ActvAssetReward { return s.rewards[group] }

// GetDraws 返回抽奖条目。
func (s *TaskLotterySettings) GetDraws() []*ActvLotteryRewardCfg { return s.draws }

// GetWeight 返回抽奖池总权重。
func (s *TaskLotterySettings) GetWeight() int64 { return s.weight }

// AllowsDrawCount 判断配置是否允许请求抽数。
func (s *TaskLotterySettings) AllowsDrawCount(count int32) bool { return s.drawCounts[count] }

type taskLotteryCatalog map[int32]*TaskLotterySettings

var taskLotterySettings atomic.Pointer[taskLotteryCatalog]

// GetTaskLotterySettings 按活动配置编号查找已校验的规则。
func GetTaskLotterySettings(id int32) *TaskLotterySettings {
	row := GetActvOnlineCfgNoErr(id)
	catalog := taskLotterySettings.Load()
	if row == nil || catalog == nil {
		return nil
	}
	return (*catalog)[row.GetContentID()]
}

// rebuildTaskLotterySettings 在依赖表均已发布后更新任务抽奖配置。
func rebuildTaskLotterySettings(_ *CsvConf) {
	main, _ := csvActvTaskLotteryCfg.Load().(*CsvConf)
	pools, _ := csvActvLotteryPoolCfg.Load().(*CsvConf)
	tasks, _ := csvActvTaskCfg.Load().(*CsvConf)
	draws, _ := csvActvLotteryRewardCfg.Load().(*CsvConf)
	rewards, _ := csvActvRewardCfg.Load().(*CsvConf)
	equipment, _ := csvBattleEquipCfg.Load().(*CsvConf)
	goods, _ := csvActvShopGoodsCfg.Load().(*CsvConf)
	if main == nil || pools == nil || tasks == nil || draws == nil || rewards == nil || equipment == nil || goods == nil {
		return
	}
	catalog := make(taskLotteryCatalog)
	rewardGroups := taskLotteryRewardGroups(rewards, equipment)
	if rewardGroups != nil {
		taskGroups := make(map[int32][]*ActvTaskCfg)
		for position := range tasks.NumRecord() {
			row := tasks.Record(position).(*ActvTaskCfg)
			group := row.GetGroupID()
			taskGroups[group] = append(taskGroups[group], row)
		}
		drawGroups := make(map[int32][]*ActvLotteryRewardCfg)
		for position := range draws.NumRecord() {
			row := draws.Record(position).(*ActvLotteryRewardCfg)
			group := row.GetGroupID()
			drawGroups[group] = append(drawGroups[group], row)
		}
		for position := range main.NumRecord() {
			row := main.Record(position).(*ActvTaskLotteryCfg)
			pool, _ := pools.Get(row.GetLotteryPoolID()).(*ActvLotteryPoolCfg)
			if row.GetUnclaimedDailyPolicy() != 1 || pool == nil {
				continue
			}
			settings := buildTaskLotterySettings(
				pool, taskGroups[row.GetTaskGroupID()], drawGroups[pool.GetRewardGroupID()],
				rewardGroups, goods,
			)
			if settings != nil {
				catalog[row.GetCfgID()] = settings
			}
		}
	}
	taskLotterySettings.Store(&catalog)
}

func taskLotteryRewardGroups(rewards, equipment *CsvConf) map[int32][]*ipb.ActvAssetReward {
	groups := make(map[int32][]*ipb.ActvAssetReward)
	for position := range rewards.NumRecord() {
		row := rewards.Record(position).(*ActvRewardCfg)
		rewardType, id, count := row.GetRewardType(), row.GetRewardID(), row.GetCount()
		if count <= 0 || count > 1000000 {
			return nil
		}
		switch rewardType {
		case 1:
			if id <= 0 {
				return nil
			}
		case 2:
			if equipment.Get(id) == nil || count > 100 {
				return nil
			}
		default:
			return nil
		}
		group := row.GetGroupID()
		groups[group] = append(groups[group], &ipb.ActvAssetReward{Type: rewardType, ID: id, Count: count})
	}
	return groups
}

func buildTaskLotterySettings(
	pool *ActvLotteryPoolCfg,
	tasks []*ActvTaskCfg,
	draws []*ActvLotteryRewardCfg,
	rewards map[int32][]*ipb.ActvAssetReward,
	goods *CsvConf,
) *TaskLotterySettings {
	cost := pool.GetCostCount()
	if len(tasks) == 0 || len(draws) == 0 || len(pool.GetDrawCounts()) == 0 ||
		pool.GetCostItemID() <= 0 || cost <= 0 || pool.GetDrawMode() != 1 {
		return nil
	}
	drawCounts := make(map[int32]bool)
	for _, count := range pool.GetDrawCounts() {
		if count <= 0 || count > 100 || drawCounts[count] || cost > math.MaxInt64/int64(count) {
			return nil
		}
		drawCounts[count] = true
	}
	for _, row := range tasks {
		goal := row.GetGoalType()
		params := row.GetGoalParams()
		if row.GetCfgID() <= 0 || goal < 1 || row.GetTarget() <= 0 || row.GetResetType() < 0 ||
			row.GetResetType() > 1 || len(rewards[row.GetRewardGroupID()]) == 0 || (goal == 1 && len(params) != 0) {
			return nil
		}
		for _, param := range params {
			if (goal == 2 && param != pool.GetCfgID()) || (goal == 3 && goods.Get(param) == nil) {
				return nil
			}
		}
	}
	var weight int64
	for _, row := range draws {
		if row.GetWeight() <= 0 || len(rewards[row.GetRewardGroupID()]) == 0 {
			return nil
		}
		weight += int64(row.GetWeight())
	}
	return &TaskLotterySettings{
		pool: pool, tasks: tasks, rewards: rewards, draws: draws, drawCounts: drawCounts, weight: weight,
	}
}

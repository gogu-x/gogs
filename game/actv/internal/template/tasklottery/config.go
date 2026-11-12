package tasklottery

import (
	"github.com/gogu-x/gogs/glconf"
	"math"
)

// settings 保存单次操作读取的已校验配置，不缓存在活动实例中。
type settings struct {
	main  *glconf.ActvTaskLotteryCfg
	pool  *glconf.ActvLotteryPoolCfg
	tasks []*glconf.ActvTaskCfg

	rewards map[int32][]*Reward
	draws   []*glconf.ActvLotteryRewardCfg
	weight  int64
}

// loadSettings 在运行时读取并校验配置引用，错误配置不会进入扣除流程。
func loadSettings(row *glconf.ActvOnlineCfg) *settings {
	if row == nil {
		return nil
	}
	main := glconf.GetActvTaskLotteryCfgNoErr(row.GetContentID())
	if main == nil || main.GetUnclaimedDailyPolicy() != 1 {
		return nil
	}
	pool := glconf.GetActvLotteryPoolCfgNoErr(main.GetLotteryPoolID())
	if pool == nil {
		return nil
	}
	s := &settings{main: main, pool: pool, rewards: make(map[int32][]*Reward)}
	for _, r := range glconf.GetAllActvRewardCfg() {
		if r.GetCount() <= 0 || r.GetCount() > 1000000 {
			return nil
		}
		switch r.GetRewardType() {
		case 1:
			if r.GetRewardID() <= 0 {
				return nil
			}
		case 2:
			if glconf.GetBattleEquipCfgNoErr(r.GetRewardID()) == nil || r.GetCount() > 100 {
				return nil
			}
		default:
			return nil
		}
		s.rewards[r.GetGroupID()] = append(s.rewards[r.GetGroupID()], &Reward{Type: r.GetRewardType(), ID: r.GetRewardID(), Count: r.GetCount()})
	}
	for _, t := range glconf.GetAllActvTaskCfg() {
		if t.GetGroupID() != main.GetTaskGroupID() {
			continue
		}
		if t.GetCfgID() <= 0 || t.GetGoalType() < 1 || t.GetTarget() <= 0 || t.GetResetType() < 0 || t.GetResetType() > 1 || !s.validRewards(t.GetRewardGroupID()) {
			return nil
		}
		if t.GetGoalType() == 1 && len(t.GetGoalParams()) != 0 {
			return nil
		}
		s.tasks = append(s.tasks, t)
	}
	for _, d := range glconf.GetAllActvLotteryRewardCfg() {
		if d.GetGroupID() != pool.GetRewardGroupID() {
			continue
		}
		if d.GetWeight() <= 0 || !s.validRewards(d.GetRewardGroupID()) {
			return nil
		}
		s.weight += int64(d.GetWeight())
		s.draws = append(s.draws, d)
	}
	if len(s.tasks) == 0 || s.weight == 0 || !s.validItem(pool.GetCostItemID()) || pool.GetCostCount() <= 0 || pool.GetDrawMode() != 1 {
		return nil
	}
	seen := map[int32]bool{}
	for _, n := range pool.GetDrawCounts() {
		if n <= 0 || n > 100 || seen[n] || pool.GetCostCount() > math.MaxInt64/int64(n) {
			return nil
		}
		seen[n] = true
	}
	if len(seen) == 0 {
		return nil
	}
	for _, t := range s.tasks {
		for _, p := range t.GetGoalParams() {
			if t.GetGoalType() == 2 && p != pool.GetCfgID() {
				return nil
			}
			if t.GetGoalType() == 3 && glconf.GetActvShopGoodsCfgNoErr(p) == nil {
				return nil
			}
		}
	}
	return s
}

// validItem 普通资源使用统一编号，不限制活动归属。
func (s *settings) validItem(id int32) bool { return id > 0 }

// validRewards 校验奖励组存在及普通资源编号。
func (s *settings) validRewards(id int32) bool {
	rs := s.rewards[id]
	if len(rs) == 0 {
		return false
	}
	for _, r := range rs {
		if r.GetType() == 1 && !s.validItem(r.GetID()) {
			return false
		}
	}
	return true
}

// loadSettings 根据活动配置编号读取当前配置，每次调用重新组装。
func (b *Behavior) loadSettings() *settings {
	return loadSettings(glconf.GetActvOnlineCfgNoErr(b.Meta().GetCfgID()))
}

//go:generate actv-router-gen
package tasklottery

import (
	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"math"
	"time"
)

// Behavior 持有同一期主活动的玩家任务、资产和兑换记录。
type Behavior struct {
	processing        uint64 // 正在等待资产回调的请求序号，零表示空闲。
	base.BaseBehavior        // 通用活动生命周期。
	players           *base.Comp[PlayerData]
	tasks             *base.TaskComp // 独立任务存盘组件。
}

// Create 绑定主活动，配置错误时阻止玩法操作。
func (*Behavior) Create(a *service.Actv, c conf.ActvConfig) service.ActvBehavior {
	b := &Behavior{}
	b.Bind(a, c)
	return b
}

// OnInit 注册玩家持久化组件和通用任务计数规则。
func (b *Behavior) OnInit() {
	b.tasks = base.NewTaskComp(2, b.Meta())
	b.players = base.NewComp[PlayerData](1)
	b.AddComp(b.players)
	b.AddComp(b.tasks)
	if b.loadSettings() == nil {
		tlog.Log.Error("[tasklottery/OnInit] 活动配置无效, actv=%v", b.Meta().GetActvID())
	}
}

// OnPlayerLoaded 加载后刷新任务状态。
func (b *Behavior) OnPlayerLoaded(uid uint64) {
	if d := b.playerData(); d != nil {
		b.refresh(uid, comm.Now(), false)
	}
}

// isOpen 只允许主活动有效时间内推进任务和抽奖。
func (b *Behavior) isOpen(now int64) bool {
	m := b.Meta()
	return !b.Actv.IsEnded() && now >= m.GetStartTs() && (m.GetEndTs() == 0 || now < m.GetEndTs())
}

// refresh 在任务刷新、登录计数和每日限购变化时标脏。
func (b *Behavior) refresh(uid uint64, now time.Time, login bool) {
	cfg := b.loadSettings()
	if cfg == nil {
		return
	}
	d := b.playerData()
	if d == nil {
		return
	}
	d.Init()
	day, _ := comm.DayBounds(now)
	if day < d.GetDayStart() {
		return
	}
	c := b.tasks.Tasks()
	if c == nil {
		return
	}
	changed := false
	if day > d.GetDayStart() {
		for _, t := range cfg.GetTasks() {
			if t.GetResetType() == 1 {
				c.Remove(uint64(t.GetCfgID()))
			}
		}

		d.DayStart = day
		changed = true
	}
	if b.isOpen(now.Unix()) {
		for _, t := range cfg.GetTasks() {
			if _, ok := c.Get(uint64(t.GetCfgID())); !ok {
				params := make([]int64, 0, len(t.GetGoalParams()))
				for _, p := range t.GetGoalParams() {
					params = append(params, int64(p))
				}
				if !c.Add(task.Instance{ID: uint64(t.GetCfgID()), ConfigID: uint32(t.GetCfgID()), Goal: uint32(t.GetGoalType()), Target: t.GetTarget(), Params: params}, now.Unix()) {
					continue
				}
				changed = true
			}
		}
		if login && d.GetLoginDay() < day {
			c.Publish(task.Event{Goal: 1, Count: 1}, now.Unix())
			d.LoginDay = day
			changed = true
		}
	}
	if changed {
		b.players.MarkDirty()
		tlog.Log.Info("[tasklottery/refresh] 刷新玩家活动, pid=%v actv=%v day=%v", uid, b.Meta().GetActvID(), day)
	}
}

// state 打包本活动的任务进度和请求状态，任务列表顺序稳定。
func (b *Behavior) state(uid uint64) *pb_actv.TaskLotteryState {
	d := b.playerData()
	if d == nil {
		return nil
	}
	_, next := comm.DayBounds(comm.Now())
	s := &pb_actv.TaskLotteryState{ActvId: uint64(b.Meta().GetActvID()), NextSequence: d.GetSequence() + 1, NextResetTime: next}
	for _, t := range b.tasks.GetData().GetTasks() {
		s.Tasks = append(s.Tasks, &pb_actv.ActvTaskState{TaskId: int32(t.GetConfigID()), Progress: t.GetProgress(), Target: t.GetTarget(), Status: int32(t.GetStatus())})
	}
	if b.processing != 0 {
		s.PendingRewards = 1
	}
	return s
}

// push 向玩家主动推送当前任务抽奖活动的数据。
func (b *Behavior) push(uid uint64) {
	if s := b.state(uid); s != nil {
		b.PushToPlayer(uid, &pb_actv.TaskLotteryNtf{Data: s})
	}
}

// query 向玩家返回当前任务抽奖活动的数据。
func (b *Behavior) query(uid uint64) {
	now := comm.Now()
	b.refresh(uid, now, false)
	ack := &pb_actv.GetTaskLotteryAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE}
	if b.loadSettings() != nil && b.Actv.GetService().IsPlayerLoaded(uid) && !b.Meta().IsExpired(now.Unix()) {
		ack.Data = b.state(uid)
		if ack.GetData() != nil {
			ack.Code = pb_common.ErrCode_OK
		}
	}
	b.PushToPlayer(uid, ack)
}

// spend 只准备通用道具扣除，实际余额由玩家侧统一校验。
func spend(tx *ipb.ActvAssetTrade, id int32, unit, count int64) bool {
	if unit <= 0 || count <= 0 || unit > math.MaxInt64/count || tx == nil {
		return false
	}
	tx.CostID = id
	tx.CostCount = unit * count
	return true
}

// grant 将已确定的奖励加入同一笔通用资产交易。
func (b *Behavior) grant(tx *ipb.ActvAssetTrade, rs []*Reward) bool {
	if tx == nil || len(rs) == 0 {
		return false
	}
	for _, r := range rs {
		if r.GetCount() <= 0 {
			return false
		}
		tx.Rewards = append(tx.GetRewards(), r.DeepCopy())
	}
	return true
}

// checkOperation 校验活动状态、处理中请求和操作序号。
func (b *Behavior) checkOperation(uid uint64, ack *pb_actv.ActvOperationAck) bool {
	seq := ack.GetSequence()
	now := comm.Now()
	if b.Actv.GetService() == nil || !b.Actv.GetService().IsPlayerLoaded(uid) || b.Meta().IsExpired(now.Unix()) {
		return false
	}
	b.refresh(uid, now, false)
	d := b.playerData()
	if d == nil {
		return false
	}
	if b.processing != 0 {
		ack.Pending = true
		if b.processing == seq {
			ack.Code = pb_common.ErrCode_OK
		}
		return false
	}
	if seq == 0 || seq != d.GetSequence()+1 || seq == math.MaxUint64 {
		ack.Code = pb_common.ErrCode_PARAM
		return false
	}
	if !b.isOpen(now.Unix()) {
		ack.Code = pb_common.ErrCode_ACTV_NOT_OPEN
		return false
	}
	return true
}

// OnLoaded 个人活动在组件加载完成后刷新任务状态。
func (b *Behavior) OnLoaded() { b.OnPlayerLoaded(uint64(b.Meta().GetOwnerID())) }

// playerData 返回当前活动数据，加载失败时阻止业务访问默认值。
func (b *Behavior) playerData() *PlayerData {
	if b.players.Blocked() {
		return nil
	}
	return b.players.GetData()
}

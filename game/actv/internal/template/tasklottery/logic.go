//go:generate actv-router-gen
package tasklottery

import (
	"crypto/rand"
	"math"
	"math/big"
	"time"

	"github.com/gogu-x/gogs/game/actv/internal/conf"
	"github.com/gogu-x/gogs/game/actv/internal/service"
	"github.com/gogu-x/gogs/game/actv/internal/template/base"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
)

// Behavior 持有同一期主活动的玩家任务、资产和兑换记录。
type Behavior struct {
	processing        uint64 // 正在等待资产回调的请求序号，零表示空闲。
	base.BaseBehavior        // 通用活动生命周期。
	players           *base.OwnedComp[PlayerData]
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
	b.players = base.NewOwnedComp[PlayerData](1, uint64(b.Meta().GetOwnerID()))
	b.AddComp(b.players)
	b.AddComp(b.tasks)
	if b.loadSettings() == nil {
		tlog.Log.Error("[tasklottery/OnInit] 活动配置无效, actv=%v", b.Meta().GetActvID())
	}
}

// OnPlayerLoaded 加载后刷新任务状态。
func (b *Behavior) OnPlayerLoaded(uid uint64) {
	if d := b.players.Get(uid); d != nil {
		b.refresh(uid, time.Now(), false)
	}
}

// handleLogin 登录按自然日计数并主动推送。
// @actv.event 2 "userLogin"
func (b *Behavior) handleLogin(event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	b.refresh(uid, time.Now(), true)
	b.push(uid)
}

// isOpen 只允许主活动有效时间内推进任务和抽奖。
func (b *Behavior) isOpen(now int64) bool {
	m := b.Meta()
	return !b.Actv.IsEnded() && now >= m.GetStartTs() && (m.GetEndTs() == 0 || now < m.GetEndTs())
}

// dayBounds 返回服务器本地自然日及下一次零点。
func dayBounds(now time.Time) (int64, int64) {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return d.Unix(), d.AddDate(0, 0, 1).Unix()
}

// refresh 在任务刷新、登录计数和每日限购变化时标脏。
func (b *Behavior) refresh(uid uint64, now time.Time, login bool) {
	cfg := b.loadSettings()
	if cfg == nil {
		return
	}
	d := b.players.Get(uid)
	if d == nil {
		return
	}
	d.Init()
	day, _ := dayBounds(now)
	if day < d.GetDayStart() {
		return
	}
	c := b.tasks.Tasks()
	if c == nil {
		return
	}
	changed := false
	if day > d.GetDayStart() {
		for _, t := range cfg.tasks {
			if t.GetResetType() == 1 {
				c.Remove(uint64(t.GetCfgID()))
			}
		}

		d.DayStart = day
		changed = true
	}
	if b.isOpen(now.Unix()) {
		for _, t := range cfg.tasks {
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
		b.players.Dirty(uid)
		tlog.Log.Info("[tasklottery/refresh] 刷新玩家活动, pid=%v actv=%v day=%v", uid, b.Meta().GetActvID(), day)
	}
}

// state 打包本活动的任务进度和请求状态，任务列表顺序稳定。
func (b *Behavior) state(uid uint64) *pb_actv.TaskLotteryState {
	d := b.players.Get(uid)
	if d == nil {
		return nil
	}
	_, next := dayBounds(time.Now())
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

// handleGet 查询也使用已加载组件，未加载时返回不可用。
// @actv.request
func (b *Behavior) handleGet(uid uint64, request *pb_actv.GetTaskLotteryReq) { b.query(uid) }

// query 向玩家返回当前任务抽奖活动的数据。
func (b *Behavior) query(uid uint64) {
	b.refresh(uid, time.Now(), false)
	ack := &pb_actv.GetTaskLotteryAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE}
	if b.loadSettings() != nil && b.Actv.GetService().IsPlayerLoaded(uid) && !b.Meta().IsExpired(time.Now().Unix()) {
		ack.Data = b.state(uid)
		if ack.GetData() != nil {
			ack.Code = pb_common.ErrCode_OK
		}
	}
	b.PushToPlayer(uid, ack)
}

// handleClaim 在通用任务领取校验通过后提交奖励。
// @actv.request
func (b *Behavior) handleClaim(uid uint64, request *pb_actv.ClaimActvTaskReq) {
	ack := &pb_actv.ActvOperationAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE, Sequence: request.GetSequence()}
	sent := false
	defer func() {
		if sent {
			return
		}
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
	}()
	if !b.checkOperation(uid, ack) {
		return
	}
	cfg := b.loadSettings()
	if cfg == nil {
		return
	}
	tx := &ipb.ActvAssetTrade{UID: uid, ActvID: b.Meta().GetActvID(), Sequence: request.GetSequence()}
	d := b.players.Get(uid)
	c := b.tasks.BindSnapshot()
	if c == nil {
		return
	}
	defer c.Close()

	var group, resetType int32
	for _, t := range cfg.tasks {
		if t.GetCfgID() == request.GetTaskId() {
			group = t.GetRewardGroupID()
			resetType = t.GetResetType()
			break
		}
	}
	if group == 0 {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	day, taskID := d.GetDayStart(), request.GetTaskId()
	rs := cfg.rewards[group]
	result := c.Claim(uint64(request.GetTaskId()), time.Now().Unix(), func(_ task.Owner, _ task.Instance) bool { return b.grant(tx, rs) })
	switch result {
	case task.ClaimOK:
		// 任务快照已完成校验，实际领取状态由资产成功回调提交。
	case task.ClaimRewarded:
		ack.Code = pb_common.ErrCode_ACTV_REWARD_ALREADY_CLAIMED
		return
	default:
		ack.Code = pb_common.ErrCode_ACTV_REWARD_NOT_READY
		return
	}
	b.processing = tx.GetSequence()
	sent = ipb.RequestActvAssets(tx, func(code pb_common.ErrCode) {
		b.processing = 0
		ack.Code = code
		d.Sequence = tx.GetSequence()
		b.refresh(uid, time.Now(), false)
		if code == pb_common.ErrCode_OK {
			if tasks := b.tasks.Tasks(); tasks != nil && (resetType == 0 || day == d.GetDayStart()) {
				tasks.Claim(uint64(taskID), time.Now().Unix(), func(task.Owner, task.Instance) bool { return true })
			}

			for _, reward := range tx.GetRewards() {
				ack.Rewards = append(ack.GetRewards(), &pb_actv.ActvRewardItem{RewardType: reward.GetType(), RewardId: reward.GetID(), Count: reward.GetCount()})
			}
		}
		b.players.Dirty(uid)
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
		b.push(uid)
		tlog.Log.Info("[tasklottery/handleClaim] 资产回调完成, pid=%v, sequence=%v, code=%v", uid, tx.GetSequence(), code)
	})
	if !sent {
		b.processing = 0
	}

}

// handleDraw 扣除单次成本乘抽数，先确定全部奖励再提交快照。
// @actv.request
func (b *Behavior) handleDraw(uid uint64, request *pb_actv.DrawActvReq) {
	ack := &pb_actv.ActvOperationAck{Code: pb_common.ErrCode_ACTV_UNAVAILABLE, Sequence: request.GetSequence()}
	sent := false
	defer func() {
		if sent {
			return
		}
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
	}()
	if !b.checkOperation(uid, ack) {
		return
	}
	cfg := b.loadSettings()
	if cfg == nil {
		return
	}
	tx := &ipb.ActvAssetTrade{UID: uid, ActvID: b.Meta().GetActvID(), Sequence: request.GetSequence()}
	d := b.players.Get(uid)

	pool := cfg.pool
	allowed := false
	for _, n := range pool.GetDrawCounts() {
		if n == request.GetCount() {
			allowed = true
		}
	}
	if !allowed {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	if !spend(tx, pool.GetCostItemID(), pool.GetCostCount(), int64(request.GetCount())) {
		ack.Code = pb_common.ErrCode_PARAM
		return
	}
	var rs []*Reward
	for i := int32(0); i < request.GetCount(); i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(cfg.weight))
		if err != nil {
			ack.Code = pb_common.ErrCode_INTERNAL
			return
		}
		v := n.Int64()
		for _, entry := range cfg.draws {
			v -= int64(entry.GetWeight())
			if v < 0 {
				rs = append(rs, cfg.rewards[entry.GetRewardGroupID()]...)
				break
			}
		}
	}
	if !b.grant(tx, rs) {
		ack.Code = pb_common.ErrCode_ACTV_UNAVAILABLE
		return
	}
	drawCount, poolID := int64(request.GetCount()), int64(pool.GetCfgID())
	b.processing = tx.GetSequence()
	sent = ipb.RequestActvAssets(tx, func(code pb_common.ErrCode) {
		b.processing = 0
		ack.Code = code
		d.Sequence = tx.GetSequence()
		b.refresh(uid, time.Now(), false)
		if code == pb_common.ErrCode_OK {
			b.Actv.GetService().HandleEvent(gameevent.NewTask(uid, task.Event{Goal: 2, Count: drawCount, Param: poolID}))

			for _, reward := range tx.GetRewards() {
				ack.Rewards = append(ack.GetRewards(), &pb_actv.ActvRewardItem{RewardType: reward.GetType(), RewardId: reward.GetID(), Count: reward.GetCount()})
			}
		}
		b.players.Dirty(uid)
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
		b.push(uid)
		tlog.Log.Info("[tasklottery/handleDraw] 资产回调完成, pid=%v, sequence=%v, code=%v", uid, tx.GetSequence(), code)
	})
	if !sent {
		b.processing = 0
	}

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
	if b.Actv.GetService() == nil || !b.Actv.GetService().IsPlayerLoaded(uid) || b.Meta().IsExpired(time.Now().Unix()) {
		return false
	}
	b.refresh(uid, time.Now(), false)
	d := b.players.Get(uid)
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
	if !b.isOpen(time.Now().Unix()) {
		ack.Code = pb_common.ErrCode_ACTV_NOT_OPEN
		return false
	}
	return true
}

// OnLoaded 个人活动在组件加载完成后刷新任务状态。
func (b *Behavior) OnLoaded() { b.OnPlayerLoaded(uint64(b.Meta().GetOwnerID())) }

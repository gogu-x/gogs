package tasklottery

import (
	"crypto/rand"
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_actv"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/task"
	"github.com/gogu-x/tree/tlog"
	"math/big"
)

// handleGet 查询也使用已加载组件，未加载时返回不可用。
// @actv.request
func (b *Behavior) handleGet(uid uint64, request *pb_actv.GetTaskLotteryReq) { b.query(uid) }

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
	d := b.playerData()
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
	result := c.Claim(uint64(request.GetTaskId()), comm.NowUnix(), func(_ task.Owner, _ task.Instance) bool { return b.grant(tx, rs) })
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
		now := comm.Now()
		b.refresh(uid, now, false)
		if code == pb_common.ErrCode_OK {
			if tasks := b.tasks.Tasks(); tasks != nil && (resetType == 0 || day == d.GetDayStart()) {
				tasks.Claim(uint64(taskID), now.Unix(), func(task.Owner, task.Instance) bool { return true })
			}

			for _, reward := range tx.GetRewards() {
				ack.Rewards = append(ack.GetRewards(), &pb_actv.ActvRewardItem{RewardType: reward.GetType(), RewardId: reward.GetID(), Count: reward.GetCount()})
			}
		}
		b.players.MarkDirty()
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
	d := b.playerData()

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
		b.refresh(uid, comm.Now(), false)
		if code == pb_common.ErrCode_OK {
			b.Actv.GetService().HandleEvent(gameevent.NewTask(uid, task.Event{Goal: 2, Count: drawCount, Param: poolID}))

			for _, reward := range tx.GetRewards() {
				ack.Rewards = append(ack.GetRewards(), &pb_actv.ActvRewardItem{RewardType: reward.GetType(), RewardId: reward.GetID(), Count: reward.GetCount()})
			}
		}
		b.players.MarkDirty()
		ack.Data = b.state(uid)
		b.PushToPlayer(uid, ack)
		b.push(uid)
		tlog.Log.Info("[tasklottery/handleDraw] 资产回调完成, pid=%v, sequence=%v, code=%v", uid, tx.GetSequence(), code)
	})
	if !sent {
		b.processing = 0
	}

}

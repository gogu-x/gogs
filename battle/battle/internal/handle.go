package internal

import (
	"fmt"
	"github.com/gogu-x/gogs/battle/robot"
	"github.com/gogu-x/gogs/natsrpc"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	common "github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// registerMessages 注册本场战斗的输入、定时唤醒及生命周期消息。
func (a *BattleActor) registerMessages() {
	tree.RegisterFunc(&a.router, (*pb.ClaimBattleGrantReq)(nil), a.onClaim)
	tree.RegisterFunc(&a.router, (*sspb.ApplyBattleGrantReq)(nil), a.onGrant)
	tree.RegisterFunc(&a.router, (*robot.Observe)(nil), a.onObserve)
	tree.RegisterFunc(&a.router, (*robot.Command)(nil), a.onRobotCommand)
	tree.RegisterFunc(&a.router, (*pb.BattleInputReq)(nil), a.onPlayerInput)
	tree.RegisterFunc(&a.router, (*statusWake)(nil), a.onStatusWake)
	tree.RegisterFunc(&a.router, (*pb.StartBattleReq)(nil), a.onJoin)
	tree.RegisterFunc(&a.router, (*beginBattle)(nil), a.onBegin)
	tree.RegisterFunc(&a.router, (*pb.BattleResultConfirmedNtf)(nil), a.onConfirm)
	tree.RegisterFunc(&a.router, (*retryTick)(nil), a.onRetry)
	tree.RegisterFunc(&a.router, (*RetireBattle)(nil), a.onRetire)
	a.router.SetFallback(func(ctx tree.Context, msg interface{}) {
		ctx.Response(nil, fmt.Errorf("battle battle %s: unsupported message %T", a.p.BattleID, msg))
	})
}

// onObserve 向机器人返回本次决策需要的战况。
func (a *BattleActor) onObserve(ctx tree.Context, msg *robot.Observe) { a.observeRobots(ctx, msg) }

// onRobotCommand 以机器人身份提交防守方动作。
func (a *BattleActor) onRobotCommand(ctx tree.Context, msg *robot.Command) {
	a.liveInput(ctx, msg.Input, true)
}

// onPlayerInput 按玩家控制权校验并执行输入。
func (a *BattleActor) onPlayerInput(ctx tree.Context, msg *pb.BattleInputReq) {
	a.liveInput(ctx, msg, false)
}

// onStatusWake 推进已经到期的战斗状态。
func (a *BattleActor) onStatusWake(ctx tree.Context, _ *statusWake) { a.liveStatus(ctx) }

// onJoin 处理发往已有战斗实例的加入请求。
func (a *BattleActor) onJoin(ctx tree.Context, msg *pb.StartBattleReq) { a.join(ctx, msg) }

// onBegin 在邮箱中启动新战斗或重投战报。
func (a *BattleActor) onBegin(ctx tree.Context, _ *beginBattle) { a.begin(ctx) }

// onConfirm 在引擎释放后接收结果确认。
func (a *BattleActor) onConfirm(ctx tree.Context, msg *pb.BattleResultConfirmedNtf) {
	if a.p.Battle == nil {
		a.confirm(ctx, msg)
	}
}

// onRetry 在确认超时后检查投递次数并处理重试。
func (a *BattleActor) onRetry(ctx tree.Context, _ *retryTick) { a.retry(ctx) }

// onRetire 回收本场战斗并通知管理器。
func (a *BattleActor) onRetire(ctx tree.Context, _ *RetireBattle) { a.finish(ctx) }

// onGrant 仅接受成员所属玩法节点发送的发放请求，返回实际入账结果。
func (a *BattleActor) onGrant(ctx tree.Context, req *sspb.ApplyBattleGrantReq) {
	ack := &sspb.ApplyBattleGrantAck{}
	owner := a.member(req.GetUid())
	if owner == nil || !owner.owns(req.GetUnitId()) || req.GetBattleId() != a.p.BattleID ||
		ctx.GetValue("sendModule") != natsrpc.Game || ctx.GetValue("id") != int32(owner.Source.ServerID) || ctx.GetValue("nodeID") != int32(owner.Source.NodeID) {
		ack.Reason = "战斗、来源节点或单位归属不匹配"
	} else if !a.advanceLive(ctx) {
		ack.Reason = "战斗已结束"
	} else if err := a.issueGrant(*owner, req); err != nil {
		ack.Reason = err.Error()
	} else {
		ack.Accepted = true
	}
	if tlog.Log != nil {
		tlog.Log.Info("[战斗/onGrant] 处理战中增益, battle=%v unit=%v grant=%v accepted=%v reason=%v pid=%v", req.GetBattleId(), req.GetUnitId(), req.GetGrant().GetGrantId(), ack.GetAccepted(), ack.GetReason(), req.GetUid())
	}
	ctx.Response(ack, nil)
}

// onClaim 按参战成员的归属和服务端候选记录执行领取。
func (a *BattleActor) onClaim(ctx tree.Context, req *pb.ClaimBattleGrantReq) {
	id, unitID := req.GetGrantId(), req.GetUnitId()
	ack := &pb.ClaimBattleGrantAck{BattleId: req.GetBattleId(), UnitId: unitID, GrantId: id, Code: common.ErrCode_INVALID_BATTLE_REQUEST}
	if m := a.member(req.GetUID()); m != nil && m.owns(unitID) && req.GetBattleId() == a.p.BattleID && a.advanceLive(ctx) {
		if err := a.claimGrant(unitID, id); err == nil {
			ack.Code = common.ErrCode_OK
		}
	}
	if tlog.Log != nil {
		tlog.Log.Info(
			"[战斗/onClaim] 处理玩家领取, battle=%v unit=%v grant=%v code=%v pid=%v",
			req.GetBattleId(), unitID, id, ack.GetCode(), req.GetUID(),
		)
	}
	ctx.Response(ack, nil)
}

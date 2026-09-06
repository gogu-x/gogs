package ctl_auth

import (
	"github.com/gogu-x/gogs/conf"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/context"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/pfpb/pb_pf"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// AutoLogin 免登录路由：建立本节点的玩家会话。
func AutoLogin(ctx *context.Context, req *pb_auth.LoginGameReq) {
	playerId := req.UID
	ack := &pb_auth.LoginGameAck{Code: pb_common.ErrCode_PARAM}
	defer ctx.CastPlayerIdMsg(playerId, ack)

	if req.UID == 0 {
		return
	}

	//触发登录事件
	arg := comm.NewArg()
	arg.Set("playerId", playerId)
	ctx.Event().Emit(context.PlayerOnLogin, arg)
	ack.Code = pb_common.ErrCode_OK
	ack.Uid = playerId

	//测试发送给pf nats消息
	err := natsrpc.Cast(natsrpc.Gate, conf.ServerID, conf.NodeId, ack)
	if err != nil {
		tlog.Log.Error("cast activity err: %v", err)
	}

	reqs := &pb_pf.AuthLoginReq{
		Account: "344343",
	}
	err = natsrpc.Call(
		natsrpc.Game,
		natsrpc.PF,
		def.PLAY,
		def.PF,
		conf.ServerID,
		conf.NodeId,
		reqs,
		ctx.Tree().Self(),
		AutoRegister,
	)
	if err != nil {
		tlog.Log.Error("cast activity err: %v", err)
	}
}

func AutoRegister(_ tree.Context, msg interface{}, err error) {
	if err != nil {
		tlog.Log.Error("auto register failed: %v", err)
		return
	}
	tlog.Log.Debug("auto register response: %T", msg)
}

package ctl_guild

import (
	"fmt"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_common"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// CreateGuild 将校验通过的玩家请求转发给全局联盟角色。
func CreateGuild(ctx *core.Context, req *pb_guild.CreateGuildReq) {
	if ctx == nil || req == nil {
		tlog.Log.Error("[guild/CreateGuild] 玩家上下文或请求为空")
		return
	}
	uid := ctx.PlayerID()
	if !ctx.CastCall(def.Guild, req, func(callbackCtx tree.Context, value interface{}, err error) {
		ack, ok := value.(*pb_guild.CreateGuildAck)
		if err != nil || !ok || ack == nil {
			ack = &pb_guild.CreateGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: callError(err, value)}
			tlog.Log.Error("[guild/CreateGuild] 创建联盟请求失败, pid=%v err=%v responseType=%T", uid, err, value)
		}
		pushAck(callbackCtx, uid, ack)
	}) {
		ctx.CastPlayerMsg(uid, &pb_guild.CreateGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: "guild actor is unavailable"})
		tlog.Log.Error("[guild/CreateGuild] 联盟 Actor 不可用, pid=%v", uid)
	}
}

// JoinGuild 将校验通过的玩家请求转发给全局联盟角色。
func JoinGuild(ctx *core.Context, req *pb_guild.JoinGuildReq) {
	if ctx == nil || req == nil {
		tlog.Log.Error("[guild/JoinGuild] 玩家上下文或请求为空")
		return
	}
	uid := ctx.PlayerID()
	if !ctx.CastCall(def.Guild, req, func(callbackCtx tree.Context, value interface{}, err error) {
		ack, ok := value.(*pb_guild.JoinGuildAck)
		if err != nil || !ok || ack == nil {
			ack = &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: callError(err, value)}
			tlog.Log.Error("[guild/JoinGuild] 加入联盟请求失败, pid=%v err=%v responseType=%T", uid, err, value)
		}
		pushAck(callbackCtx, uid, ack)
	}) {
		ctx.CastPlayerMsg(uid, &pb_guild.JoinGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: "guild actor is unavailable"})
		tlog.Log.Error("[guild/JoinGuild] 联盟 Actor 不可用, pid=%v", uid)
	}
}

// LeaveGuild 将校验通过的玩家请求转发给全局联盟角色。
func LeaveGuild(ctx *core.Context, req *pb_guild.LeaveGuildReq) {
	if ctx == nil || req == nil {
		tlog.Log.Error("[guild/LeaveGuild] 玩家上下文或请求为空")
		return
	}
	uid := ctx.PlayerID()
	if !ctx.CastCall(def.Guild, req, func(callbackCtx tree.Context, value interface{}, err error) {
		ack, ok := value.(*pb_guild.LeaveGuildAck)
		if err != nil || !ok || ack == nil {
			ack = &pb_guild.LeaveGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: callError(err, value)}
			tlog.Log.Error("[guild/LeaveGuild] 离开联盟请求失败, pid=%v err=%v responseType=%T", uid, err, value)
		}
		pushAck(callbackCtx, uid, ack)
	}) {
		ctx.CastPlayerMsg(uid, &pb_guild.LeaveGuildAck{Code: pb_common.ErrCode_INTERNAL, Msg: "guild actor is unavailable"})
		tlog.Log.Error("[guild/LeaveGuild] 联盟 Actor 不可用, pid=%v", uid)
	}
}

// GetGuild 将校验通过的玩家请求转发给全局联盟角色。
func GetGuild(ctx *core.Context, req *pb_guild.GetGuildReq) {
	if ctx == nil || req == nil {
		tlog.Log.Error("[guild/GetGuild] 玩家上下文或请求为空")
		return
	}
	uid := ctx.PlayerID()
	if !ctx.CastCall(def.Guild, req, func(callbackCtx tree.Context, value interface{}, err error) {
		ack, ok := value.(*pb_guild.GetGuildAck)
		if err != nil || !ok || ack == nil {
			ack = &pb_guild.GetGuildAck{Code: pb_common.ErrCode_INTERNAL}
			tlog.Log.Error("[guild/GetGuild] 查询联盟请求失败, pid=%v err=%v responseType=%T", uid, err, value)
		}
		pushAck(callbackCtx, uid, ack)
	}) {
		ctx.CastPlayerMsg(uid, &pb_guild.GetGuildAck{Code: pb_common.ErrCode_INTERNAL})
		tlog.Log.Error("[guild/GetGuild] 联盟 Actor 不可用, pid=%v", uid)
	}
}

func callError(err error, value interface{}) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("invalid guild response %T", value)
}

func pushAck(ctx tree.Context, uid uint64, ack interface{}) {
	gatePID, ok := ctx.Lookup(def.GameGate)
	if !ok {
		tlog.Log.Error("[guild/pushAck] 网关 Actor 不可用, pid=%v", uid)
		return
	}
	if !ctx.Send(gatePID, &ipb.PushToMsg{UID: uid, Msg: ack}) {
		tlog.Log.Error("[guild/pushAck] 联盟响应投递失败, pid=%v", uid)
	}
}

package ctl

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/pb/protoGuild"
)

func CreateGuild(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGuild.CreateGuildReq)
	req.Uid = a.Player.UID
	req.LeaderName = a.Player.Name
	req.LeaderLevel = a.Player.Level

	requestGuild(a, ctx, req, func(ret interface{}, err error) {
		if err != nil {
			a.Reply(&protoGuild.CreateGuildResp{Code: -1, Msg: err.Error()})
			return
		}
		a.Reply(ret.(*protoGuild.CreateGuildResp))
	})
}

func JoinGuild(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGuild.JoinGuildReq)
	req.Uid = a.Player.UID
	req.MemberName = a.Player.Name
	req.MemberLevel = a.Player.Level

	requestGuild(a, ctx, req, func(ret interface{}, err error) {
		if err != nil {
			a.Reply(&protoGuild.JoinGuildResp{Code: -1, Msg: err.Error()})
			return
		}
		a.Reply(ret.(*protoGuild.JoinGuildResp))
	})
}

func LeaveGuild(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := &protoGuild.LeaveGuildReq{Uid: a.Player.UID}

	requestGuild(a, ctx, req, func(ret interface{}, err error) {
		if err != nil {
			a.Reply(&protoGuild.LeaveGuildResp{Code: -1, Msg: err.Error()})
			return
		}
		a.Reply(ret.(*protoGuild.LeaveGuildResp))
	})
}

func GetGuild(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoGuild.GetGuildReq)
	requestGuild(a, ctx, req, func(ret interface{}, err error) {
		if err != nil {
			a.Reply(&protoGuild.GetGuildResp{Code: -1})
			return
		}
		a.Reply(ret.(*protoGuild.GetGuildResp))
	})
}

func requestGuild(a *app.App, ctx actor.ActorContext, msg interface{ ProtoMessage() }, cb func(interface{}, error)) {
	ctx.Request(actor.MustLookup(constant.ActorGuild), msg).Callback(ctx, func(ret interface{}, err error) {
		if err != nil {
			log.Printf("requestGuild error: %v", err)
		}
		cb(ret, err)
	})
}

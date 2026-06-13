package ctl

import (
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/game/app"
	"github.com/gogu-x/gogs/game/constant"
	"github.com/gogu-x/gogs/pb/protoActivity"
)

func GetActivityList(a *app.App, ctx actor.ActorContext, msg interface{}) {
	requestActivity(a, ctx, msg.(*protoActivity.GetActivityListReq), func(ret interface{}, err error) {
		a.Reply(ret.(*protoActivity.GetActivityListResp))
	})
}

func JoinActivity(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoActivity.JoinActivityReq)
	req.Uid = a.Player.UID

	requestActivity(a, ctx, req, func(ret interface{}, err error) {
		a.Reply(ret.(*protoActivity.JoinActivityResp))
	})
}

func GetProgress(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoActivity.GetProgressReq)
	req.Uid = a.Player.UID

	requestActivity(a, ctx, req, func(ret interface{}, err error) {
		a.Reply(ret.(*protoActivity.GetProgressResp))
	})
}

func ClaimReward(a *app.App, ctx actor.ActorContext, msg interface{}) {
	req := msg.(*protoActivity.ClaimRewardReq)
	req.Uid = a.Player.UID

	requestActivity(a, ctx, req, func(ret interface{}, err error) {
		a.Reply(ret.(*protoActivity.ClaimRewardResp))
	})
}

func requestActivity(a *app.App, ctx actor.ActorContext, msg interface{ ProtoMessage() }, cb func(interface{}, error)) {
	ctx.Request(actor.MustLookup(constant.ActorActivity), msg).Callback(ctx, func(ret interface{}, err error) {
		if err != nil {
			log.Printf("requestActivity error: %v", err)
		}
		cb(ret, err)
	})
}

package ctl_actv

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// forwardActvRequest 只投递请求，由 Actv 直接向 Gate 发送结果。
func forwardActvRequest(
	ctx *core.Context,
	actvID uint64,
	request proto.Message,
) {
	uid := ctx.PlayerID()
	if uid == 0 || ctx.TreeCtx == nil || request == nil || !request.ProtoReflect().IsValid() {
		tlog.Log.Error("[ctl_actv/forwardActvRequest] 活动请求上下文无效, pid=%v", uid)
		return
	}
	generation := ctx.SessionGeneration()
	message := &ipb.PlayerActvMessage{
		UID: uid, ActvID: actvID, Generation: generation,
		Online: ctx.SessionActive(), Message: request,
	}
	actvPID, ok := ctx.TreeCtx.Lookup(def.ActorActv)
	if !ok || !ctx.TreeCtx.TrySend(actvPID, message) {
		tlog.Log.Error("[ctl_actv/forwardActvRequest] 活动服务不可用或队列已满, pid=%v request=%T", uid, request)
	}
}

package actv

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// actvReplySession 提供回复目标和会话版本，供独立列表消息与业务消息共用。
type actvReplySession interface {
	GetUID() uint64
	GetGeneration() uint64
}

// pushActv 仅在具体处理函数主动调用时推送，保留请求的会话版本。
func pushActv(ctx tree.Context, message actvReplySession, response proto.Message) bool {
	uid := message.GetUID()
	if ctx == nil || uid == 0 || response == nil || !response.ProtoReflect().IsValid() {
		return false
	}
	gatePID, ok := ctx.Lookup(def.GameGate)
	if !ok || !ctx.Send(gatePID, &ipb.PushToMsg{UID: uid, Generation: message.GetGeneration(), Msg: response}) {
		tlog.Log.Error("[actv/pushActv] 活动消息发送失败, pid=%v", uid)
		return false
	}
	return true
}

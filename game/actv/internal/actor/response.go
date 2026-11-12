package actor

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// pushActv 仅在具体处理函数主动调用时推送，保留请求的会话版本。
func pushActv(ctx tree.Context, message *ipb.PlayerActvMessage, response proto.Message) bool {
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

// logActvFailure 记录加载或投递失败，不替业务构造客户端消息。
func logActvFailure(message *ipb.PlayerActvMessage, err error) {
	tlog.Log.Error("[actv/logActvFailure] 活动请求不可用, pid=%v err=%v", message.GetUID(), err)
}

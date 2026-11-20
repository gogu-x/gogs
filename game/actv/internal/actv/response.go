package actv

import (
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// actvReplySession 提供回复目标和会话版本，供独立列表消息与业务消息共用。
type actvReplySession interface {
	GetUID() uint64
	GetGeneration() uint64
}

// pushActv 仅在具体处理函数主动调用时推送，保留请求的会话版本。
//
// 按命名规则查找玩家会话代理并直接投递，不经过 Gate；玩家离线或代理队列已满时丢弃本条
// 推送，不阻塞全区共用的 Actv。会话已被替换时，新代理按版本丢弃旧请求的响应。
func pushActv(ctx tree.Context, message actvReplySession, response proto.Message) bool {
	uid := message.GetUID()
	if ctx == nil || uid == 0 || response == nil || !response.ProtoReflect().IsValid() {
		return false
	}
	return ipb.PushToSession(ctx, uid, message.GetGeneration(), response)
}

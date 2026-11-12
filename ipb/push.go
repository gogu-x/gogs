package ipb

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// PushToPlayer 向进程 Gate 投递玩家消息，由 Gate 按 UID 选择当前在线会话。
// 不捕获请求上下文或会话版本，适用于请求处理、定时器和异步完成后的主动推送。
// 返回 true 仅表示已入 Gate 队列；离线玩家不会补发。调用后不得修改 message。
func PushToPlayer(uid uint64, message proto.Message) bool {
	if uid == 0 || message == nil || !message.ProtoReflect().IsValid() {
		return false
	}
	gatePID, ok := tree.Lookup(def.GameGate)
	if !ok || !tree.Default().TrySend(gatePID, &PushToMsg{UID: uid, Msg: message}) {
		tlog.Error("[ipb/PushToPlayer] Gate 不可用或队列已满, pid=%v", uid)
		return false
	}
	return true
}

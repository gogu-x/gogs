package ipb

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

// SessionSender 是直接推送所需的最小 Actor 能力；tree.Context 满足该接口。
type SessionSender interface {
	Lookup(name string) (tree.PID, bool)
	TrySend(pid tree.PID, msg interface{}) bool
}

// globalSender 供不在 Actor 上下文内的调用方使用全局 Actor 注册表。
type globalSender struct{}

func (globalSender) Lookup(name string) (tree.PID, bool)        { return tree.Lookup(name) }
func (globalSender) TrySend(pid tree.PID, msg interface{}) bool { return tree.TrySend(pid, msg) }

// PushToSession 按命名规则查找玩家当前会话代理并直接投递，不经过游戏网关。
//
// generation 非 0 时，代理只写出与自身会话版本一致的消息，避免旧请求的响应写到新连接；
// 传 0 表示推送给当前会话。玩家不在线时返回 false 且不记录错误；代理队列已满时请求网关
// 关闭该会话并返回 false。投递为非阻塞，调用后不得修改 message。
func PushToSession(sender SessionSender, uid uint64, generation uint64, message interface{}) bool {
	if sender == nil || uid == 0 || message == nil {
		return false
	}
	agentPID, online := sender.Lookup(def.SessionAgentName(uid))
	if !online {
		return false
	}
	if sender.TrySend(agentPID, &PushToMsg{UID: uid, Generation: generation, Msg: message}) {
		return true
	}
	tlog.Log.Error("[ipb/PushToSession] 会话代理不可用或出站队列已满, pid=%v agentPID=%v type=%T", uid, agentPID, message)
	requestCloseSession(sender, uid, agentPID, "outbound mailbox is full")
	return false
}

// PushToPlayer 在 Actor 上下文之外向玩家当前会话直接推送消息，语义同 PushToSession。
// 适用于请求处理、定时器和异步完成后的主动推送；离线玩家不会补发。
func PushToPlayer(uid uint64, message proto.Message) bool {
	if message == nil || !message.ProtoReflect().IsValid() {
		return false
	}
	return PushToSession(globalSender{}, uid, 0, message)
}

// requestCloseSession 请求游戏网关关闭指定代理的会话；代理已被替换时网关忽略该请求。
func requestCloseSession(sender SessionSender, uid uint64, agentPID tree.PID, reason string) {
	gatePID, ok := sender.Lookup(def.GameGate)
	if !ok || !sender.TrySend(gatePID, &CloseSession{UID: uid, AgentPID: agentPID, Reason: reason}) {
		tlog.Log.Error("[ipb/requestCloseSession] 请求网关关闭会话失败, pid=%v agentPID=%v reason=%v", uid, agentPID, reason)
	}
}

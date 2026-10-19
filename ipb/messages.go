// Package ipb 定义游戏 Actor 之间共用的进程内消息。
package ipb

import "github.com/gogu-x/tree"

// PushToMsg 将编码后的客户端协议消息发送到指定玩家的在线会话。
type PushToMsg struct {
	UID uint64      // 目标玩家 ID。
	Msg interface{} // 要推送给客户端的协议消息。
}

// BanUID 将玩家加入当前网关的内存封禁列表。
type BanUID struct {
	UID uint64 // 被封禁的玩家 ID。
}

// UnbanUID 将玩家从当前网关的内存封禁列表移除。
type UnbanUID struct {
	UID uint64 // 被解封的玩家 ID。
}

// SessionClosed 通知玩家 Actor 网关会话已关闭。
type SessionClosed struct {
	UID        uint64 // 玩家唯一 ID。
	Generation uint64 // 本次网关会话版本。
}

// GetUID 返回会话已关闭的玩家编号。
func (message *SessionClosed) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration 返回已关闭会话的版本号。
func (message *SessionClosed) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// PlayerSessionOpened 将新建的网关会话绑定到玩家 Actor。
type PlayerSessionOpened struct {
	UID        uint64 // 玩家唯一 ID。
	Generation uint64 // 本次网关会话版本。
}

// GetUID 返回会话已打开的玩家编号。
func (message *PlayerSessionOpened) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration 返回已打开会话的版本号。
func (message *PlayerSessionOpened) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// PlayerInbound 携带客户端请求及其网关会话版本。
type PlayerInbound struct {
	UID        uint64      // 玩家唯一 ID。
	Generation uint64      // 发送该请求的网关会话版本。
	Message    interface{} // 客户端业务请求。
}

// GetUID 返回发送该请求的玩家编号。
func (message *PlayerInbound) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration 返回发送该请求的会话版本号。
func (message *PlayerInbound) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// GetMessage 返回该消息携带的客户端请求。
func (message *PlayerInbound) GetMessage() interface{} {
	if message == nil {
		return nil
	}
	return message.Message
}

// SavePlayer 请求玩家 Actor 存盘当前内存数据。
type SavePlayer struct {
	UID  uint64 // 玩家唯一 ID；0 表示 Actor 自己的玩家。
	Wait bool   // 为 true 时，Actor 在持久化完成后回复请求。
}

// StopAcceptingPlayer 请求玩家 Actor 在关服前拒绝新的客户端请求。
type StopAcceptingPlayer struct {
	UID uint64 // 玩家唯一 ID。
}

// SaveAllGuilds 请求联盟 Actor 存盘所有已变更的联盟数据。
type SaveAllGuilds struct{}

// SaveAllActivities 请求活动 Actor 存盘所有已变更的活动数据。
type SaveAllActivities struct{}

// PlayerActivityMessage 将玩家活动请求从玩家 Actor 转发到全局活动 Actor。
type PlayerActivityMessage struct {
	UID        uint64      // 玩家唯一 ID。
	Generation uint64      // 当前网关会话版本。
	Online     bool        // 转发请求时玩家是否在线。
	Message    interface{} // 要处理的活动业务请求。
}

// GetUID 返回活动请求所属玩家的编号。
func (message *PlayerActivityMessage) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration 返回转发活动请求时使用的网关会话版本。
func (message *PlayerActivityMessage) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// GetOnline 返回转发活动请求时玩家是否在线。
func (message *PlayerActivityMessage) GetOnline() bool {
	return message != nil && message.Online
}

// GetMessage 返回需要处理的活动业务请求。
func (message *PlayerActivityMessage) GetMessage() interface{} {
	if message == nil {
		return nil
	}
	return message.Message
}

// PlayerActivityOwnerReleased 通知活动 Actor 保存并释放离线玩家的活动数据。
type PlayerActivityOwnerReleased struct {
	UID uint64 // 玩家唯一 ID。
}

// GetUID 返回需要释放活动数据的玩家编号。
func (message *PlayerActivityOwnerReleased) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGuildActivities 请求联盟 Actor 返回玩家所属联盟的活动。
type GetGuildActivities struct {
	UID uint64 // 查询联盟活动的玩家 ID。
}

// GetUID 返回查询联盟活动的玩家编号。
func (message *GetGuildActivities) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// StopPlayerActor 请求玩家 Actor 存盘并在满足条件后停止。
type StopPlayerActor struct {
	UID      uint64 // 玩家唯一 ID。
	Shutdown bool   // 为 true 时关闭会话并在完成存盘后停止 Actor。
}

// GetUID 返回停止请求对应的玩家 ID。
func (message *StopPlayerActor) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetShutdown 返回该请求是否由服务关服流程发起。
func (message *StopPlayerActor) GetShutdown() bool {
	return message != nil && message.Shutdown
}

// PlayerMessage 通过网关玩家目录向指定玩家投递服务器业务命令。
type PlayerMessage struct {
	UID     uint64      // 命令目标玩家 ID。
	Message interface{} // 要交给玩家 Actor 处理的业务消息。
}

// PlayerActorEvictReady 通知网关离线玩家 Actor 已存盘并可以停止。
type PlayerActorEvictReady struct {
	UID uint64   // 玩家唯一 ID。
	PID tree.PID // 已完成存盘的玩家 Actor PID。
}

// PlayerActorStopped 通知网关玩家 Actor 已停止。
type PlayerActorStopped struct {
	UID uint64   // 玩家唯一 ID。
	PID tree.PID // 已停止的玩家 Actor PID。
}

// PlayerLoadFailed 向网关会话管理者报告玩家数据加载失败。
type PlayerLoadFailed struct {
	UID    uint64   // 玩家唯一 ID。
	PID    tree.PID // 加载失败的玩家 Actor PID。
	Reason string   // 加载失败原因。
}

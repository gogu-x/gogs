// Package ipb defines in-process messages shared by game actors.
package ipb

import "github.com/gogu-x/tree"

// PushToMsg routes an encoded client payload to the live session identified by UID.
// Payload uses tree/codec.ProtoCodec framing and can be forwarded to the client as-is.
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

// SessionClosed notifies the player's Actor that its gateway session has closed.
type SessionClosed struct {
	UID        uint64 // 玩家唯一 ID。
	Generation uint64 // 本次网关会话版本。
}

// GetUID returns the player whose session closed.
func (message *SessionClosed) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration returns the closed session generation.
func (message *SessionClosed) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// PlayerSessionOpened binds a newly opened gateway session to its PlayerActor.
type PlayerSessionOpened struct {
	UID        uint64 // 玩家唯一 ID。
	Generation uint64 // 本次网关会话版本。
}

// GetUID returns the player whose session opened.
func (message *PlayerSessionOpened) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration returns the opened session generation.
func (message *PlayerSessionOpened) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// PlayerInbound carries a client request tagged with its gateway session generation.
type PlayerInbound struct {
	UID        uint64      // 玩家唯一 ID。
	Generation uint64      // 发送该请求的网关会话版本。
	Message    interface{} // 客户端业务请求。
}

// GetUID returns the player whose session sent this request.
func (message *PlayerInbound) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGeneration returns the session generation that sent this request.
func (message *PlayerInbound) GetGeneration() uint64 {
	if message == nil {
		return 0
	}
	return message.Generation
}

// GetMessage returns the client request carried by this message.
func (message *PlayerInbound) GetMessage() interface{} {
	if message == nil {
		return nil
	}
	return message.Message
}

// SavePlayer asks the player's Actor to persist its current in-memory snapshot.
type SavePlayer struct {
	UID  uint64 // 玩家唯一 ID；0 表示 Actor 自己的玩家。
	Wait bool   // 为 true 时，Actor 在持久化完成后回复请求。
}

// StopAcceptingPlayer asks a player's Actor to reject further client requests before shutdown.
type StopAcceptingPlayer struct {
	UID uint64 // 玩家唯一 ID。
}

// SaveAllGuilds asks the Guild Actor to persist every dirty guild aggregate.
type SaveAllGuilds struct{}

// SaveAllActivities asks the Activity Actor to persist server activity state.
type SaveAllActivities struct{}

// SavePlayerActivities asks one player's activity Actor to persist its record.
type SavePlayerActivities struct {
	UID uint64 // 玩家唯一 ID；0 表示 Actor 自己的玩家。
}

// GetUID returns the target player's ID, or zero for the current player.
func (message *SavePlayerActivities) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// PlayerActivityMessage routes an offline server command to a player's activity Actor.
type PlayerActivityMessage struct {
	UID     uint64      // 命令目标玩家 ID。
	Message interface{} // 要交给玩家活动 Actor 的业务消息。
}

// GetUID returns the player targeted by this activity command.
func (message *PlayerActivityMessage) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetMessage returns the activity command to dispatch.
func (message *PlayerActivityMessage) GetMessage() interface{} {
	if message == nil {
		return nil
	}
	return message.Message
}

// GetGuildActivities asks the Guild Actor for activities owned by a player's guild.
type GetGuildActivities struct {
	UID uint64 // 查询联盟活动的玩家 ID。
}

// GetUID returns the player whose guild activities are requested.
func (message *GetGuildActivities) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// StopPlayerActor asks a PlayerActor to save its data and stop when eligible.
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

// PlayerMessage routes a server-side player command through the gateway's player directory.
type PlayerMessage struct {
	UID     uint64      // 命令目标玩家 ID。
	Message interface{} // 要交给玩家 Actor 处理的业务消息。
}

// PlayerActorEvictReady reports that an offline player Actor has saved and can be stopped.
type PlayerActorEvictReady struct {
	UID uint64   // 玩家唯一 ID。
	PID tree.PID // 已完成存盘的玩家 Actor PID。
}

// PlayerActorStopped reports that a player Actor has stopped.
type PlayerActorStopped struct {
	UID uint64   // 玩家唯一 ID。
	PID tree.PID // 已停止的玩家 Actor PID。
}

// PlayerLoadFailed reports a player data load failure to the gateway session owner.
type PlayerLoadFailed struct {
	UID    uint64   // 玩家唯一 ID。
	PID    tree.PID // 加载失败的玩家 Actor PID。
	Reason string   // 加载失败原因。
}

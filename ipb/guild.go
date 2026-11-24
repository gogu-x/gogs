package ipb

// SyncPlayerGuild 请求从联盟成员索引同步玩家归属。
type SyncPlayerGuild struct {
	UID uint64 // 要同步联盟归属的玩家 ID。
}

// GetUID 返回待同步的玩家编号。
func (message *SyncPlayerGuild) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// PlayerGuildMembership 通知玩家 Actor 更新并持久化联盟归属。
type PlayerGuildMembership struct {
	UID     uint64 // 玩家唯一 ID。
	GuildID uint64 // 当前所属联盟 ID，0 表示无联盟。
}

// GetUID 返回联盟归属对应的玩家编号。
func (message *PlayerGuildMembership) GetUID() uint64 {
	if message == nil {
		return 0
	}
	return message.UID
}

// GetGuildID 返回已确认的联盟归属。
func (message *PlayerGuildMembership) GetGuildID() uint64 {
	if message == nil {
		return 0
	}
	return message.GuildID
}

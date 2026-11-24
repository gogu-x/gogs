package guild

import "github.com/gogu-x/gogs/game/guild/internal"

// Guild 是管理联盟数据的 Actor。
type Guild = internal.GuildActor

// NewGuild 创建全局联盟 Actor。
func NewGuild() *Guild {
	return internal.NewGuildActor()
}

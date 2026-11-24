package ctl_guild

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/tlog"
)

// SyncOnLogin 从联盟成员索引同步当前玩家的联盟编号。
func SyncOnLogin(actor *core.PlayerActor, _ *comm.Arg) {
	current := actor.GetPlayer()
	uid := current.GetUID()
	if uid == 0 {
		return
	}
	guildPID, ok := tree.Lookup(def.Guild)
	if !ok || !tree.TrySend(guildPID, &ipb.SyncPlayerGuild{UID: uid}) {
		tlog.Log.Error("[ctl_guild/SyncOnLogin] 玩家联盟归属同步请求未投递, pid=%v", uid)
	}
}

// OnMembership 在玩家 Actor 内更新所属联盟编号。
func OnMembership(ctx *core.Context, message *ipb.PlayerGuildMembership) {
	current := ctx.GetPlayer()
	uid := message.GetUID()
	if uid == 0 || current.GetUID() != uid {
		tlog.Log.Error("[ctl_guild/OnMembership] 玩家联盟归属与当前玩家不匹配, pid=%v", uid)
		return
	}
	before := current.GetGuildID()
	guildID := message.GetGuildID()
	if current.SetGuildID(guildID) {
		tlog.Log.Info("[ctl_guild/OnMembership] 玩家联盟归属已更新, pid=%v before=%v after=%v", uid, before, guildID)
	}
}

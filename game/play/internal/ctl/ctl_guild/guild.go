package ctl_guild

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
)

// Create 为当前玩家创建联盟。
func Create(ctx *core.Context, req *pb_guild.CreateGuildReq) {
	CreateGuild(ctx, req)
}

// Join 将当前玩家加入联盟。
func Join(ctx *core.Context, req *pb_guild.JoinGuildReq) {
	JoinGuild(ctx, req)
}

// Leave 将当前玩家移出其所属联盟。
func Leave(ctx *core.Context, req *pb_guild.LeaveGuildReq) {
	LeaveGuild(ctx, req)
}

// Get 返回联盟数据快照。
func Get(ctx *core.Context, req *pb_guild.GetGuildReq) {
	GetGuild(ctx, req)
}

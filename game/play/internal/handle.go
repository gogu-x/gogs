package internal

import (
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_actv"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_battle"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_equipment"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_guild"
	"github.com/gogu-x/gogs/pb/cspb/pb_activity"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/gogs/pb/cspb/pb_guild"
)

// InitRoutes 注册单个玩家角色的全部业务路由。
func InitRoutes(actor *core.PlayerActor) {
	core.RegisterSysMsg(actor, (*pb_auth.LoginGameReq)(nil), ctl_auth.AutoLogin)
	core.RegisterPlayerMsg(actor, (*pb_activity.GetActivityListReq)(nil), ctl_actv.OnGetActivityList)
	core.RegisterPlayerMsg(actor, (*pb_activity.JoinActivityReq)(nil), ctl_actv.OnJoinActivity)
	core.RegisterPlayerMsg(actor, (*pb_activity.GetProgressReq)(nil), ctl_actv.OnGetActivityProgress)
	core.RegisterPlayerMsg(actor, (*pb_activity.ClaimRewardReq)(nil), ctl_actv.OnClaimActivityReward)
	core.RegisterPlayerMsg(actor, (*pb_battle.StartBattleReq)(nil), ctl_battle.OnCreateBattle)
	core.RegisterPlayerMsg(actor, (*pb_equipment.GetRoleEquipmentReq)(nil), ctl_equipment.OnGetRoleEquipment)
	core.RegisterPlayerMsg(actor, (*pb_equipment.ChangeRoleEquipmentReq)(nil), ctl_equipment.OnChangeRoleEquipment)
	core.RegisterPlayerMsg(actor, (*pb_guild.CreateGuildReq)(nil), ctl_guild.Create)
	core.RegisterPlayerMsg(actor, (*pb_guild.JoinGuildReq)(nil), ctl_guild.Join)
	core.RegisterPlayerMsg(actor, (*pb_guild.LeaveGuildReq)(nil), ctl_guild.Leave)
	core.RegisterPlayerMsg(actor, (*pb_guild.GetGuildReq)(nil), ctl_guild.Get)
	core.RegisterSysMsg(actor, (*battle.Settlement)(nil), ctl_battle.OnBattleFinished)
}

package internal

import (
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_battle"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_equipment"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_equipment"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
)

// InitRoutes 注册 play 模块所有路由。
func InitRoutes(py *core.Play) {
	py.Router().Register((*ipb.SessionClosed)(nil), onSessionClosed(py))
	core.RegisterSysMsg(py, (*pb_auth.LoginGameReq)(nil), ctl_auth.AutoLogin)
	core.RegisterPlayerMsg(py, (*pb_battle.StartBattleReq)(nil), ctl_battle.OnCreateBattle)
	core.RegisterPlayerMsg(py, (*pb_equipment.GetRoleEquipmentReq)(nil), ctl_equipment.OnGetRoleEquipment)
	core.RegisterPlayerMsg(py, (*pb_equipment.ChangeRoleEquipmentReq)(nil), ctl_equipment.OnChangeRoleEquipment)
	core.RegisterSysMsg(py, (*battle.Settlement)(nil), ctl_battle.OnBattleFinished)
}

// onSessionClosed 网关会话断开时清理本节点的在线玩家。
func onSessionClosed(py *core.Play) tree.Handler {
	return func(_ tree.Context, msg interface{}) {
		closed := msg.(*ipb.SessionClosed)
		arg := comm.NewArg()
		arg.Set("uid", closed.UID)
		py.Emit(core.PlayerOnLogout, arg)
	}
}

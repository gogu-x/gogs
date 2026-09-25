package internal

import (
	"github.com/gogu-x/gogs/game/battle"
	"github.com/gogu-x/gogs/game/play/internal/core"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_auth"
	"github.com/gogu-x/gogs/game/play/internal/ctl/ctl_battle"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/gogs/pb/cspb/pb_auth"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
)

// InitRoutes 注册 play 模块所有路由。
func InitRoutes(py *core.Play) {
	py.Router().Register((*ipb.SessionClosed)(nil), onSessionClosed(py))
	core.RegisterSysMsg(py, (*pb_auth.LoginGameReq)(nil), ctl_auth.AutoLogin)
	core.RegisterPlayerMsg(py, (*pb_battle.StartBattleReq)(nil), ctl_battle.OnCreateBattle)
	core.RegisterSysMsg(py, (*battle.Settlement)(nil), ctl_battle.OnBattleFinished)
}

// onSessionClosed 网关会话断开时清理本节点的在线玩家。
func onSessionClosed(py *core.Play) tree.Handler {
	return func(_ tree.Context, msg interface{}) {
		closed := msg.(*ipb.SessionClosed)
		if py.PlayerMgr.Remove(closed.UID) != nil {
			tlog.Log.Info("play: uid=%d session removed, online=%d", closed.UID, py.PlayerMgr.Count())
		}
	}
}

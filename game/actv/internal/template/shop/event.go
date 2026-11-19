package shop

import (
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/comm"
)

// handleLogin 登录刷新并推送商店。
// @actv.event 2 "userLogin"
func (b *Shop) handleLogin(event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	b.refresh(uid, comm.Now())
	b.push(uid)
}

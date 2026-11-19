package tasklottery

import (
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/comm"
)

// handleLogin 登录按自然日计数并主动推送。
// @actv.event 2 "userLogin"
func (b *Behavior) handleLogin(event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	b.refresh(uid, comm.Now(), true)
	b.push(uid)
}

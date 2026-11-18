package tasklottery

import (
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
	"time"
)

// handleLogin 登录按自然日计数并主动推送。
// @actv.event 2 "userLogin"
func (b *Behavior) handleLogin(event *sspb.Event) {
	uid := gameevent.PlayerUID(event)
	b.refresh(uid, time.Now(), true)
	b.push(uid)
}

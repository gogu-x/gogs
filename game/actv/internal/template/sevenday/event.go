package sevenday

import (
	gameevent "github.com/gogu-x/gogs/game/event"
	"github.com/gogu-x/gogs/pb/sspb"
)

// handleLogin 消费玩家登录事件，直接使用事件发生时间计数。
// @actv.event 2 "userLogin"
func (b *Behavior) handleLogin(event *sspb.Event) {
	if gameevent.PlayerUID(event) != uint64(b.Meta().GetOwnerID()) {
		return
	}
	generation := gameevent.PlayerGeneration(event)
	if generation == 0 || generation <= b.generation {
		return
	}
	b.generation = generation
	b.updateOnline(true, gameevent.PlayerEventTime(event))
}

// handleLogout 消费玩家离线事件，跨零点时先结算再离线。
// @actv.event 2 "userLogout"
func (b *Behavior) handleLogout(event *sspb.Event) {
	if gameevent.PlayerUID(event) != uint64(b.Meta().GetOwnerID()) {
		return
	}
	if generation := gameevent.PlayerGeneration(event); generation != b.generation {
		return
	}
	b.updateOnline(false, gameevent.PlayerEventTime(event))
}

// handleRelease 在玩家释放前结束在线状态，不另建会话回调接口。
// @actv.event 2 "playerReleased"
func (b *Behavior) handleRelease(event *sspb.Event) { b.handleLogout(event) }

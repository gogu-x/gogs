package tasklottery

import "time"

// OnTicker 跨日刷新已加载的个人任务，不累计登录次数。
func (b *Behavior) OnTicker() {
	now := time.Now()
	day, _ := dayBounds(now)
	uid := uint64(b.Meta().GetOwnerID())
	if d := b.players.Get(uid); d != nil && d.GetDayStart() < day {
		b.refresh(uid, now, false)
		b.push(uid)
	}
}

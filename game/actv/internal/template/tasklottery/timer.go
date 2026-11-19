package tasklottery

import "github.com/gogu-x/tree/comm"

// OnTicker 跨日刷新已加载的个人任务，不累计登录次数。
func (b *Behavior) OnTicker() {
	now := comm.Now()
	day, _ := comm.DayBounds(now)
	uid := uint64(b.Meta().GetOwnerID())
	if d := b.playerData(); d != nil && d.GetDayStart() < day {
		b.refresh(uid, now, false)
		b.push(uid)
	}
}

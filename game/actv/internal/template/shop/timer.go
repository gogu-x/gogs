package shop

import "github.com/gogu-x/tree/comm"

// OnTicker 每日重置购买次数。
func (b *Shop) OnTicker() {
	uid := uint64(b.Meta().GetOwnerID())
	d := b.playerData()
	now := comm.Now()
	day, _ := comm.DayBounds(now)
	if d != nil && day > d.GetDayStart() {
		b.refresh(uid, now)
		b.push(uid)
	}
}

package shop

import "time"

// OnTicker 每日重置购买次数。
func (b *Shop) OnTicker() {
	uid := uint64(b.Meta().GetOwnerID())
	d := b.data.Get(uid)
	day, _ := period(time.Now())
	if d != nil && day > d.GetDayStart() {
		b.refresh(uid, time.Now())
		b.push(uid)
	}
}

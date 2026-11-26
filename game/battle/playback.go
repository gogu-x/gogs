package battle

import (
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// playbackWindow 将逻辑时间映射为下一场战斗的最早准入时间。
type playbackWindow struct {
	firstMS int64     // 首个事件的毫秒时间，保持首个动作立即播放的约定。
	anchor  time.Time // 首个事件接收时间。
	readyAt time.Time // 当前已知事件覆盖的结束时间。
}

// newPlaybackWindow 创建演出窗口，使用战斗经过毫秒计算事件间隔。
func newPlaybackWindow() *playbackWindow { return &playbackWindow{} }

// observe 记录最终逻辑时间和动作窗口，计算期间不等待真实时间。
func (p *playbackWindow) observe(event *pb.BattleEvent, now time.Time) {
	if p == nil || event == nil {
		return
	}
	elapsedMS := event.GetElapsedMs()
	if p.anchor.IsZero() {
		p.anchor = now
		p.firstMS = elapsedMS
	}
	if end := event.GetEndMs(); end > elapsedMS {
		elapsedMS = end
	}
	if elapsedMS < p.firstMS {
		return
	}
	readyAt := p.anchor.Add(time.Duration(elapsedMS-p.firstMS) * time.Millisecond)
	if readyAt.After(p.readyAt) {
		p.readyAt = readyAt
	}
}

// pending 判断演出窗口是否尚未结束，结束通知提前到达也不能绕过准入。
func (p *playbackWindow) pending(now time.Time) bool {
	return p != nil && now.Before(p.readyAt)
}

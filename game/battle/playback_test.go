package battle

import (
	"testing"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// TestPlaybackWindowUsesLogicalTime 验证首帧立即播放、动作窗口和最终事件延长准入期限。
func TestPlaybackWindowUsesLogicalTime(t *testing.T) {
	start := time.Unix(1000, 0)
	window := newPlaybackWindow(100)
	window.observe(&pb.BattleEvent{Tick: 10, EndTick: 16}, start)
	window.observe(&pb.BattleEvent{Tick: 20}, start.Add(time.Millisecond))
	if !window.pending(start.Add(999 * time.Millisecond)) {
		t.Fatal("事件尚未播放完就允许下一场")
	}
	if window.pending(start.Add(time.Second)) {
		t.Fatal("演出结束后仍禁止下一场")
	}
	window.observe(&pb.BattleEvent{Tick: 13}, start.Add(2*time.Second))
	if window.pending(start.Add(time.Second)) {
		t.Fatal("较早事件不应改变播放锚点")
	}
}

package battle

import (
	"testing"
	"time"

	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
)

// TestPlaybackWindowUsesLogicalTime 验证首个事件立即播放、动作窗口和最终事件延长准入期限。
func TestPlaybackWindowUsesLogicalTime(t *testing.T) {
	start := time.Unix(1000, 0)
	window := newPlaybackWindow()
	window.observe(&pb.BattleEvent{ElapsedMs: 1000, EndMs: 1600}, start)
	window.observe(&pb.BattleEvent{ElapsedMs: 2000}, start.Add(time.Millisecond))
	if !window.pending(start.Add(999 * time.Millisecond)) {
		t.Fatal("事件尚未播放完就允许下一场")
	}
	if window.pending(start.Add(time.Second)) {
		t.Fatal("演出结束后仍禁止下一场")
	}
	window.observe(&pb.BattleEvent{ElapsedMs: 1300}, start.Add(2*time.Second))
	if window.pending(start.Add(time.Second)) {
		t.Fatal("较早事件不应改变播放锚点")
	}
}

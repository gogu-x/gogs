package event

import (
	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree/task"
	"testing"
)

// TestTaskOwnerParsing 验证不同归属的任务参数以及非法归属的拒绝。
func TestTaskOwnerParsing(t *testing.T) {
	progress := task.Event{Goal: 4, Count: 2, Param: 9}
	for _, message := range []*sspb.Event{NewTask(7, progress), NewGuildTask(7, progress), NewServerTask(progress)} {
		if got, ok := ReadTask(message); !ok || got != progress {
			t.Fatal("任务参数解析失败")
		}
	}
	for _, message := range []*sspb.Event{nil, NewTask(0, progress), NewGuildTask(0, progress), NewGuildTask(uint64(1)<<63, progress), NewServerTask(task.Event{})} {
		if _, ok := ReadTask(message); ok {
			t.Fatal("接受无效任务事件")
		}
	}
}

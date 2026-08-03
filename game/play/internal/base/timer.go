package base

import (
	"time"
)

const saveInterval = 5 * time.Minute

// InitTimers 在 PlayerActor.OnInit 中调用，注册所有定时任务
func InitTimers(s *PlayContext) {
	scheduleSave(s)
}

func scheduleSave(s *PlayContext) {
	s.AfterFunc(saveInterval, func() {
		s.Data.Save()
		scheduleSave(s)
	})
}

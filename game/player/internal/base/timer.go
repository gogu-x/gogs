package base

import (
	"time"
)

const saveInterval = 5 * time.Minute

// InitTimers 在 PlayerActor.OnInit 中调用，注册所有定时任务
func InitTimers(s *Session) {
	scheduleSave(s)
}

func scheduleSave(s *Session) {
	s.AfterFunc(saveInterval, func() {
		s.Data.Save()
		scheduleSave(s)
	})
}

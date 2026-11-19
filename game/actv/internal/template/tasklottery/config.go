package tasklottery

import "github.com/gogu-x/gogs/glconf"

func (b *Behavior) loadSettings() *glconf.TaskLotterySettings {
	return glconf.GetTaskLotterySettings(b.Meta().GetCfgID())
}

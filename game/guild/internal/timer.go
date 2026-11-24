package internal

import (
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/timer"
)

// guildSaveTickerType 是联盟定时存盘事件的类型。
const guildSaveTickerType = timer.TimerType(1000003)

// initTimer 创建联盟 Actor 使用的时间轮。
func (guild *GuildActor) initTimer(ctx tree.Context) {
	guild.wheel = timer.NewTimeWheel(32, ctx.Self())
}

// stopTimer 停止已创建的时间轮。
func (guild *GuildActor) stopTimer() {
	if wheel := guild.wheel; wheel != nil {
		wheel.Stop()
	}
}

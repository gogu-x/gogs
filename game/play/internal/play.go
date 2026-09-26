// Package internal 负责 play 模块的装配：把业务包（ctl_*）的处理函数注册到
// // core.Play 上。装配天生需要认识全部业务包，而业务包需要认识 core.Context，
// // 因此装配与 Play 的定义必须分包，否则会形成 internal → ctl_* → core → internal 的循环。
package internal

import (
	"github.com/gogu-x/gogs/game/play/internal/core"
)

// NewPlay 构造 Play Actor，并注入装配逻辑。
// Boot 由 core.Play.OnInit 在公共能力就绪之后调用，此时
func NewPlay() *core.Play {
	py := &core.Play{}
	py.Boot = func(p *core.Play) {
		InitTimers(p)
		InitEvent(p)
		InitRoutes(p)
		RegDbLoader(p)
	}
	return py
}

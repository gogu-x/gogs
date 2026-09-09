// Package core 提供 play 模块的运行期状态（Play）与单次请求上下文（Context）。
//
// core 位于 play 依赖图的最底层：它不引用任何 ctl_* 业务包，也不引用上层
// internal 包，因此所有业务包都可以自由 import 而不产生循环依赖。
// 路由、事件监听与定时任务的注册属于「装配」职责，天生需要认识全部业务包，
// 因此留在上层 internal，通过 Play.Boot 钩子注入。
package core

import (
	"time"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
)

// timeWheelChanLen 是时间轮到期队列的缓冲大小。
const timeWheelChanLen = 16

// Play 是游戏逻辑 Actor，持有本节点的运行期状态与公共能力。
//
// 所有字段只在 Play 自己的 Actor goroutine 内访问，无需加锁。
type Play struct {
	PlayerMgr *player.PlayerMgr
	Event     *comm.Event
	TimeWheel *timer.TimeWheel

	router tree.Router
	// Boot 在公共能力就绪之后、开服事件触发之前被调用，由上层装配时注入，
	Boot func(*Play)
}

func (py *Play) Name() string { return def.PLAY }

func (py *Play) OnInit(ctx tree.Context) {
	py.PlayerMgr = player.NewPlayerMgr()
	py.Event = comm.NewEvent()
	py.TimeWheel = timer.NewTimeWheel(timeWheelChanLen, ctx.Self(), ctx.System())

	// 装配必须在能力就绪之后：Boot 内的注册逻辑会直接使用 Event / TimeWheel。
	if py.Boot != nil {
		py.Boot(py)
	}
	py.Emit(ServerStart, comm.NewArg())
}

func (py *Play) HandleMessage(ctx tree.Context, msg interface{}) {
	py.router.Route(ctx, msg)
}

func (py *Play) OnStop(_ tree.Context) {
	if py.TimeWheel != nil {
		py.TimeWheel.Stop()
	}
}

// Router 供上层注册路由。tree.Router 的方法是指针接收者，故返回指针。
func (py *Play) Router() *tree.Router { return &py.router }

// EventFunc 是事件监听的业务签名。ctx 由框架注入：请求中触发的
// 事件复用请求 Context，开服等系统事件使用不带请求信息的 SysCtx。
type EventFunc func(play *Play, arg *comm.Arg)

func (py *Play) OnEvent(name comm.EventName, fn EventFunc, priority ...int) {
	if fn == nil {
		return
	}
	py.Event.Register(name, func(arg *comm.Arg) {
		fn(py, arg)
	}, priority...)
}

// Emit always starts a system event scope. Context.Emit should be used while
// handling a request so sender/request metadata is retained.
func (py *Play) Emit(name comm.EventName, arg *comm.Arg) int {
	return py.Event.Emit(name, arg)
}

// ---------- 定时器 ----------

type TimerFunc func(play *Play, data interface{})

func (py *Play) OnTimer(timerType timer.TimerType, fn TimerFunc) {
	if fn == nil {
		return
	}
	py.TimeWheel.Register(timerType, func(data interface{}) {
		fn(py, data)
	})
}

// After 挂一条一次性延时任务，到期回调在本 Actor goroutine 内执行。
// 周期任务在回调末尾再次调用 After 即可。
func (py *Play) After(timerType timer.TimerType, d time.Duration, data interface{}) *timer.WheelTimer {
	return py.TimeWheel.After(timerType, d, data)
}

// Cron 按 cron 表达式挂一条周期任务。
func (py *Play) Cron(timerType timer.TimerType, expr *timer.CronExpr, data interface{}) *timer.WheelCron {
	return py.TimeWheel.Cron(timerType, expr, data)
}

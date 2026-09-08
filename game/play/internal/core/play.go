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
	ctx    tree.Context

	// Boot 在公共能力就绪之后、开服事件触发之前被调用，由上层装配时注入，
	// 用于注册路由、事件监听与定时任务。
	Boot func(*Play)
}

func (py *Play) Name() string { return def.PLAY }

func (py *Play) OnInit(ctx tree.Context) {
	py.ctx = ctx
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

// BootCtx 返回 OnInit 期捕获的 Actor context，仅可用于取 Self() / System()。
//
// 它不携带任何请求信息（sender / request / values 均为零值），因此
// 处理请求时一律使用 Context.TreeCtx，否则 Response 会静默失效。
func (py *Play) BootCtx() tree.Context { return py.ctx }

// SysCtx 返回 Actor 级上下文，供定时器、事件与启停流程使用。

// EventFunc 是事件监听的业务签名。ctx 由框架注入：
// 事件在请求处理中触发时是那条请求的 Context，否则是 SysCtx。
type EventFunc func(py *Play, arg *comm.Arg)

// OnEvent 注册事件监听。相比直接使用 Play.Event.Register，
// 它会在分发时注入正确的 *Context，业务无需自行捕获能力。
func (py *Play) OnEvent(name comm.EventName, fn EventFunc, priority ...int) {
	if fn == nil {
		return
	}
	py.Event.Register(name, func(arg *comm.Arg) {
		fn(py, arg)
	}, priority...)
}

// Emit 在无请求上下文的场景（开服、定时器之外的内部流程）同步触发事件，
// 监听者拿到 SysCtx。请求处理中请用 Context.Emit 以保留请求上下文。
func (py *Play) Emit(name comm.EventName, arg *comm.Arg) int {
	return py.emit(name, arg)
}

// emit 在 ctx 绑定期间同步分发事件，结束后恢复原值以支持事件嵌套触发。
func (py *Play) emit(name comm.EventName, arg *comm.Arg) int {
	return py.Event.Emit(name, arg)
}

// ---------- 定时器 ----------

// TimerFunc 是定时任务的业务签名。ctx 恒为 SysCtx：定时器到期没有请求方，
// 因此 ctx.Response 是空操作，但其余能力齐全。
type TimerFunc func(py *Play, data interface{})

// OnTimer 注册 timerType 对应的定时回调，分发时注入 SysCtx。
// 必须在 Play 的 Actor goroutine 内调用（即 Boot 期间）。
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

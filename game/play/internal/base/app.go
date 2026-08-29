package base

import (
	"log"
	"reflect"

	player2 "github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
)

// App 是整个 play 模块的状态载体：管理器集合 + 路由表 + Actor 级上下文。
// 它定义在 base 而不是 play 包，这样 PlayContext / SysContext 可以直接引用它
// 而不产生 play → internal → base → play 的 import 环。
type App struct {
	Players *player2.PlayerMgr

	// Sender Frame 出口，默认走 NATS，单测可替换。
	Sender Sender

	disp Dispatcher

	timeWheel *timer.TimeWheel

	event *comm.Event
}

func NewApp() *App {
	return &App{
		Players: player2.NewPlayerMgr(),
		Sender:  natsSender{},
		event:   comm.NewEvent(),
	}
}

// Dispatcher 返回路由表，供 InitRoutes 注册。
func (a *App) Dispatcher() *Dispatcher { return &a.disp }

// Event 返回模块事件分发器。
func (a *App) Event() *comm.Event { return a.event }

// Init 在 Play Actor 的 OnInit 中调用：初始化 Actor 资源、加载数据并启动定时任务。
func (a *App) Init(ctx tree.Context) {
	a.timeWheel = timer.NewTimeWheel(16, ctx.Self(), ctx.System())
	InitTimers(a)
	InitEven(a)
	log.Printf("play: app ready, players=%d, playerRoutes=%d, sysRoutes=%d",
		a.Players.Count(), a.disp.PlayerRouteCount(), a.disp.SysRouteCount())
}

// Stop 在 Play Actor 的 OnStop 中调用。
func (a *App) Stop() {
	if a.timeWheel != nil {
		a.timeWheel.Stop()
	}
	a.Players.Save()
}

// HandleFrame 处理 gate 玩家请求：解码 → 查路由 → 登录校验 → 构造 PlayContext → 分发。
// 返回 false 表示该 Frame 未被处理（未知消息、未注册路由或未登录）。
func (a *App) HandleFrame(ctx tree.Context, f *protoGateway.Frame) bool {
	if f.GetMsgType() == natsrpc.MsgTypeDisconnect || len(f.GetPayload()) == 0 {
		a.onDisconnect(f)
		return true
	}

	msg, err := codec.ProtoCodec.Unmarshal(f.GetPayload())
	if err != nil {
		log.Printf("play: decode frame from uid=%d error: %v", f.GetUid(), err)
		return false
	}

	r, ok := a.disp.lookupPlayer(reflect.TypeOf(msg))
	if !ok {
		log.Printf("play: no player route for %T (uid=%d)", msg, f.GetUid())
		return false
	}

	var p *player2.Player
	if !r.anonymous {
		if p = a.Players.Get(f.GetUid()); p == nil {
			// 未登录（或已被踢下线）请求受保护消息：丢弃，不回包，避免被伪造 uid 放大。
			log.Printf("play: reject %T, uid=%d not online", msg, f.GetUid())
			return false
		}
	}

	s := &PlayContext{
		SysContext: SysContext{App: a, ctx: ctx},
		Player:     p,
		PlayerId:   f.GetUid(),
		ConnID:     f.GetConnId(),
		GateId:     f.GetGateId(),
		RequestId:  f.GetRequestId(),
		frame:      f,
	}
	r.h(s, msg)
	return true
}

// HandleSystem 处理其他模块投递 / 内部异步消息，不做任何登录校验。
// 返回 false 表示没有注册对应路由。
func (a *App) HandleSystem(ctx tree.Context, msg interface{}) bool {
	h, ok := a.disp.lookupSys(reflect.TypeOf(msg))
	if !ok {
		return false
	}
	h(&SysContext{App: a, ctx: ctx}, msg)
	return true
}

// onDisconnect 客户端断线：移除在线玩家并落库。
func (a *App) onDisconnect(f *protoGateway.Frame) {
	p := a.Players.Remove(f.GetUid())
	if p == nil {
		return
	}
	log.Printf("play: uid=%d disconnected, online=%d", f.GetUid(), a.Players.Count())
	// TODO: 单个玩家落库（等 PlayerMgr.Save 拆出按 uid 的版本后接上）
}

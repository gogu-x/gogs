package internal

import (
	"log"

	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/game/play/internal/context"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
	"github.com/gogu-x/gogs/ipb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/comm"
	"github.com/gogu-x/tree/timer"
)

// Play 分发玩家和系统消息。
type Play struct {
	PlayerMgr *player.PlayerMgr
	timeWheel *timer.TimeWheel
	event     *comm.Event
	router    tree.Router
	ctx       tree.Context
}

func (py *Play) Name() string { return def.PLAY }

func (py *Play) OnInit(ctx tree.Context) {
	py.PlayerMgr = player.NewPlayerMgr()
	py.event = comm.NewEvent()
	py.ctx = ctx
	InitTimers(py)
	InitEvent(py)
	InitRoutes(py)
	//服务器启动
	py.Event().Emit(context.ServerStart, comm.NewArg())
}

func (py *Play) HandleMessage(ctx tree.Context, msg interface{}) {
	py.router.Route(ctx, msg)
}

func (py *Play) OnStop(_ tree.Context) {
	if py.timeWheel != nil {
		py.timeWheel.Stop()
	}
}

func (py *Play) Event() *comm.Event { return py.event }

func (py *Play) TreeCtx() tree.Context { return py.ctx }

func (py *Play) Services() *context.Services {
	return &context.Services{
		Players:   py.PlayerMgr,
		Event:     py.event,
		TimeWheel: py.timeWheel,
		TreeCtx:   py.ctx,
	}
}

func (py *Play) onSessionClosed(_ tree.Context, msg interface{}) {
	closed := msg.(*ipb.SessionClosed)
	if py.PlayerMgr.Remove(closed.UID) != nil {
		log.Printf("play: uid=%d session removed, online=%d", closed.UID, py.PlayerMgr.Count())
	}
}

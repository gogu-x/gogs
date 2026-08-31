package internal

import (
	"log"

	"github.com/gogu-x/gogs/constant"
	gamemsg "github.com/gogu-x/gogs/game/message"
	"github.com/gogu-x/gogs/game/play/internal/common"
	"github.com/gogu-x/gogs/game/play/internal/module/player"
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

func NewPlay() *Play {
	return &Play{
		PlayerMgr: player.NewPlayerMgr(),
		event:     comm.NewEvent(),
	}
}

func (py *Play) Name() string { return constant.PLAY }

func (py *Play) OnInit(ctx tree.Context) {
	py.ctx = ctx
	InitTimers(py)
	InitEvent(py)
	InitRoutes(py)
	py.router.Register(&gamemsg.SessionClosed{}, py.onSessionClosed)
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

func (py *Play) Services() *common.Services {
	return &common.Services{
		Players:   py.PlayerMgr,
		Event:     py.event,
		TimeWheel: py.timeWheel,
	}
}

func (py *Play) onSessionClosed(_ tree.Context, msg interface{}) {
	closed := msg.(*gamemsg.SessionClosed)
	if py.PlayerMgr.Remove(closed.UID) != nil {
		log.Printf("play: uid=%d session removed, online=%d", closed.UID, py.PlayerMgr.Count())
	}
}

package play

import (
	"log"

	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/game/play/internal"
	"github.com/gogu-x/gogs/game/play/internal/base"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
)

// Play 游戏模块 Actor 壳：只负责生命周期与消息分流，状态与路由都在 base.App。
type Play struct {
	app *base.App
}

func NewPlay() *Play {
	return &Play{app: base.NewApp()}
}

// Name Play 以固定名字寻址，Spawn 时即写入 registry。
func (p *Play) Name() string { return constant.PLAY }

func (p *Play) OnInit(ctx tree.Context) {
	internal.InitRoutes(p.app)

	p.app.Init(ctx)
}

// HandleMessage 按入口分流：
func (p *Play) HandleMessage(ctx tree.Context, msg interface{}) {
	//这里需要先执行钩子handle
	if f, ok := msg.(*protoGateway.Frame); ok {
		p.app.HandleFrame(ctx, f)
		return
	}
	if !p.app.HandleSystem(ctx, msg) {
		log.Printf("play: no system route for %T", msg)
	}
}

func (p *Play) OnStop(_ tree.Context) {
	p.app.Stop()
}

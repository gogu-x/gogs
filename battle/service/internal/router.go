package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// InitRoutes 同风格：按具体消息类型分发，未注册类型由 fallback 给出错误。
func (s *Server) initRouter() {
	tree.RegisterFunc(&s.router, (*pb_battle.StartBattleReq)(nil), s.OnCreateBattle)
	tree.RegisterFunc(&s.router, (*battle.ActorStopped)(nil), s.OnActorStopped)

	s.router.SetFallback(func(ctx tree.Context, msg interface{}) {
		ctx.Response(nil, fmt.Errorf("battle service: unsupported message %T", msg))
	})
}

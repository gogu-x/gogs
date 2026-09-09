package internal

import (
	"fmt"

	"github.com/gogu-x/gogs/battle/battle/internal"
	pb "github.com/gogu-x/gogs/pb/cspb/pb_battle"
	"github.com/gogu-x/tree"
)

// initRouter 把 manager battle 的消息路由注册到 s.router。与 game 各模块的
// InitRoutes 同风格：按具体消息类型分发，未注册类型由 fallback 给出错误。
func (s *Service) initRouter() {
	tree.RegisterFunc(&s.router, (*pb.StartBattleReq)(nil), s.start)
	tree.RegisterFunc(&s.router, (*pb.QueryBattleReq)(nil), s.query)
	tree.RegisterFunc(&s.router, (*pb.RebuildBattleReq)(nil), s.rebuild)
	tree.RegisterFunc(&s.router, (*pb.VerifyBattleReplayReq)(nil), s.verify)
	tree.RegisterFunc(&s.router, (*pb.BattleResultConfirmedNtf)(nil), s.confirm)
	tree.RegisterFunc(&s.router, (*QueryPlayerRecent)(nil), s.queryRecent)
	tree.RegisterFunc(&s.router, (*internal.ActorStopped)(nil), func(ctx tree.Context, m *internal.ActorStopped) {
		s.release(m)
	})
	s.router.SetFallback(func(ctx tree.Context, msg interface{}) {
		ctx.Response(nil, fmt.Errorf("battle service: unsupported message %T", msg))
	})
}

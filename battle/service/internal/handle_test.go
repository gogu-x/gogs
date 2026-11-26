package internal

import "github.com/gogu-x/tree"

// OnInit 在真实管理器路由上追加测试查询，保证读取与活跃登记更新串行。
func (s *observedServer) OnInit(ctx tree.Context) {
	s.Server.OnInit(ctx)
	tree.RegisterFunc(&s.router, (*activeBattleQuery)(nil), s.onActiveBattle)
}

// onActiveBattle 返回指定战斗当前登记的 Actor 编号。
func (s *observedServer) onActiveBattle(ctx tree.Context, msg *activeBattleQuery) {
	ctx.Response(s.active[msg.battleID], nil)
}

package internal

import (
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/tree"
)

// Server 是 battle 进程内的 manager battle：只负责 battle 编排、登记与只读
type Server struct {
	router tree.Router
	self   tree.PID
	system *tree.Tree
	active map[string]tree.PID
}

func New() *Server {
	return &Server{
		active: make(map[string]tree.PID),
	}
}

func (s *Server) Name() string     { return def.BattleService }
func (s *Server) MailboxSize() int { return 1024 }

func (s *Server) OnInit(ctx tree.Context) {
	s.self, s.system = ctx.Self(), ctx.System()
	s.initRouter()
}

func (s *Server) OnStop(_ tree.Context) {}

func (s *Server) HandleMessage(ctx tree.Context, msg interface{}) {
	s.router.Route(ctx, msg)
}

package internal

import (
	"github.com/gogu-x/gogs/battle/battle"
	"github.com/gogu-x/gogs/def"
	"github.com/gogu-x/gogs/natsrpc"
	"github.com/gogu-x/tree"
	"google.golang.org/protobuf/proto"
)

// Server 是 battle 进程内的 Manager：只管理 BattleActor 生命周期。
type Server struct {
	router   tree.Router
	self     tree.PID
	system   *tree.Tree
	active   map[string]tree.PID
	reports  battle.Repository
	notifier battle.Notifier
}

func New() *Server {
	return &Server{
		active:  make(map[string]tree.PID),
		reports: battle.NewMemoryRepository(),
		notifier: battle.NotifyFunc(func(source battle.Source, message proto.Message) error {
			return natsrpc.Cast(natsrpc.Game, def.BattleClient, source.ServerID, source.NodeID, message)
		}),
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

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
	active   map[string]tree.PID
	compiler *battle.ConfigCompiler
	reports  battle.Repository
	notifier battle.Notifier
}

// New 创建战斗服务实例。
func New() *Server {
	return &Server{
		active:   make(map[string]tree.PID),
		compiler: battle.NewDefaultConfigCompiler(),
		reports:  battle.NewMemoryRepository(),
		notifier: battle.NotifyFunc(func(source battle.Source, message proto.Message) error {
			return natsrpc.Cast(natsrpc.Game, def.BattleClient, source.ServerID, source.NodeID, message)
		}),
	}
}

// Name 返回 Actor 的注册名称。
func (s *Server) Name() string { return def.BattleService }

// MailboxSize 返回战斗消息邮箱的容量。
func (s *Server) MailboxSize() int { return 1024 }

// OnInit 初始化消息路由及运行所需的资源。
func (s *Server) OnInit(ctx tree.Context) {
	s.self = ctx.Self()
	s.initRouter()
}

// OnStop 响应管理服务停止回调，无需额外清理。
func (s *Server) OnStop(_ tree.Context) {}

// HandleMessage 将邮箱消息交给已注册的路由。
func (s *Server) HandleMessage(ctx tree.Context, msg interface{}) {
	s.router.Route(ctx, msg)
}

package ws

import (
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
)

func initRouter(s *Server) {
	s.router.Register(&protoGateway.ConnRegMsg{}, s.handleReg)
	s.router.Register(&protoGateway.ConnUnregMsg{}, s.handleUnreg)
	s.router.Register(&protoGateway.BroadcastMsg{}, s.handleBroadcast)
}

func (s *Server) handleReg(_ tree.Context, msg interface{}) {
	s.clients[msg.(*protoGateway.ConnRegMsg).ConnId] = struct{}{}
}

func (s *Server) handleUnreg(_ tree.Context, msg interface{}) {
	delete(s.clients, msg.(*protoGateway.ConnUnregMsg).ConnId)
}

func (s *Server) handleBroadcast(_ tree.Context, msg interface{}) {
	m := msg.(*protoGateway.BroadcastMsg)
	for connID := range s.clients {
		if pid, ok := tree.Lookup(constant.ConnName(connID)); ok {
			tree.Send(pid, m)
		}
	}
}

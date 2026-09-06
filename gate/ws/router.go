package ws

import (
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
)

func initRouter(s *Server) {
	s.router.Register(&pb_gateway.ConnRegMsg{}, s.handleReg)
	s.router.Register(&pb_gateway.ConnUnregMsg{}, s.handleUnreg)
	s.router.Register(&pb_gateway.BroadcastMsg{}, s.handleBroadcast)
}

func (s *Server) handleReg(_ tree.Context, msg interface{}) {
	s.clients[msg.(*pb_gateway.ConnRegMsg).ConnId] = struct{}{}
}

func (s *Server) handleUnreg(_ tree.Context, msg interface{}) {
	delete(s.clients, msg.(*pb_gateway.ConnUnregMsg).ConnId)
}

func (s *Server) handleBroadcast(_ tree.Context, msg interface{}) {
	m := msg.(*pb_gateway.BroadcastMsg)
	for connID := range s.clients {
		if pid, ok := tree.Lookup(constant.ConnName(connID)); ok {
			tree.Send(pid, m)
		}
	}
}

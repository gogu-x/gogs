package wsserver

import (
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
)

func initRouter(s *Server) {
	s.router.Register(&protoGateway.ConnRegMsg{}, s.handleReg)
	s.router.Register(&protoGateway.ConnUnregMsg{}, s.handleUnreg)
	s.router.Register(&protoGateway.BroadcastMsg{}, s.handleBroadcast)
}

func (s *Server) handleReg(_ actor.ActorContext, msg interface{}) {
	s.conns[msg.(*protoGateway.ConnRegMsg).ConnId] = struct{}{}
}

func (s *Server) handleUnreg(_ actor.ActorContext, msg interface{}) {
	delete(s.conns, msg.(*protoGateway.ConnUnregMsg).ConnId)
}

func (s *Server) handleBroadcast(_ actor.ActorContext, msg interface{}) {
	m := msg.(*protoGateway.BroadcastMsg)
	for connID := range s.conns {
		if pid, ok := actor.Lookup(constant.ConnName(connID)); ok {
			actor.Send(pid, m)
		}
	}
}

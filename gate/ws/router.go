package ws

import (
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func initRouter(s *Server) {
	s.router.Register(&pb_gateway.ConnRegMsg{}, s.handleReg)
	s.router.Register(&pb_gateway.ConnUnregMsg{}, s.handleUnreg)
	s.router.Register(&pb_gateway.BroadcastMsg{}, s.handleBroadcast)
	s.router.Register(&constant.GetServerGrpcClientReq{}, s.handleGrpcClient)
}

func (s *Server) handleReg(_ tree.Context, msg interface{}) {
	s.Clients[msg.(*pb_gateway.ConnRegMsg).ConnId] = struct{}{}
}

func (s *Server) handleUnreg(_ tree.Context, msg interface{}) {
	delete(s.Clients, msg.(*pb_gateway.ConnUnregMsg).ConnId)
}

func (s *Server) handleBroadcast(_ tree.Context, msg interface{}) {
	m := msg.(*pb_gateway.BroadcastMsg)
	for connID := range s.Clients {
		if pid, ok := tree.Lookup(constant.ConnName(connID)); ok {
			tree.Send(pid, m)
		}
	}
}

// 获取grpc连接
func (s *Server) handleGrpcClient(ctx tree.Context, msg interface{}) {
	req, ok := msg.(*constant.GetServerGrpcClientReq)
	ack := &constant.GetServerGrpcClientAck{}
	evle := ctx.RequestEnvelope()
	if !ok {
		evle.Respond(ack, nil)
		return
	}
	if s.GameGrpcPoolMgr[req.ServerId] != nil {
		ack.Grpc = s.GameGrpcPoolMgr[req.ServerId]
		evle.Respond(ack, nil)
		return
	}
	addr, err := cluster.GetAddr(req.ServerId)
	if err != nil {
		evle.Respond(ack, err)
		return
	}
	grpcConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		evle.Respond(ack, err)
		return
	}
	s.GameGrpcPoolMgr[req.ServerId] = grpcConn
	ack.Grpc = grpcConn
	evle.Respond(ack, nil)

}

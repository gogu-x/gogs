package model

import (
	"fmt"
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/pb/gateway"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type inboundMsg struct {
	msg    proto.Message
	uid    uint64
	connID uint64
}

type GateActor struct {
	grpcServer *grpc.Server
	gamePID    actor.PID
	sys        *actor.ActorSystem
	stream     gateway.Gateway_StreamServer
}

func (g *GateActor) OnInit(ctx actor.ActorContext) {
	g.gamePID, _ = ctx.Lookup("game")
	g.sys = ctx.System()

	lis, err := net.Listen("tcp", config.GrpcAddr())
	if err != nil {
		log.Fatalf("GateActor: listen error: %v", err)
	}

	g.grpcServer = grpc.NewServer()
	gateway.RegisterGatewayServer(g.grpcServer, &gatewayService{actor: g, codec: codec.JsonCodec})

	go func() {
		log.Printf("GateActor: gRPC server listening on %s", config.GrpcAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			log.Printf("GateActor: grpc serve error: %v", err)
		}
	}()

	if err := cluster.Register(fmt.Sprintf("%d", config.ServerID), config.GrpcAddr()); err != nil {
		log.Printf("GateActor: cluster register error: %v", err)
	} else {
		log.Printf("GateActor: registered [%d] -> %s", config.ServerID, config.GrpcAddr())
	}
}

func (g *GateActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *inboundMsg:
		ctx.Send(g.gamePID, m)
	case *gateway.Frame:
		if g.stream != nil {
			if err := g.stream.Send(m); err != nil {
				log.Printf("GateActor: stream send error: %v", err)
			}
		}
	}
}

func (g *GateActor) OnStop(ctx actor.ActorContext) {
	if g.grpcServer != nil {
		g.grpcServer.GracefulStop()
	}
}

type gatewayService struct {
	gateway.UnimplementedGatewayServer
	actor *GateActor
	codec codec.Codec
}

func (s *gatewayService) Stream(stream gateway.Gateway_StreamServer) error {
	s.actor.stream = stream
	self := s.actor.sys.MustLookup("gate")
	sys := s.actor.sys

	for {
		frame, err := stream.Recv()
		if err != nil {
			s.actor.stream = nil
			return err
		}
		if len(frame.Payload) == 0 {
			continue
		}
		inner, err := s.codec.Unmarshal(frame.Payload)
		if err != nil {
			log.Printf("gatewayService: unmarshal error: %v", err)
			continue
		}
		protoMsg, ok := inner.(proto.Message)
		if !ok {
			log.Printf("inner: proto.Message error: %v", err)
			continue
		}
		sys.Send(self, &inboundMsg{
			msg:    protoMsg,
			uid:    frame.Uid,
			connID: frame.ConnId,
		})
	}
}

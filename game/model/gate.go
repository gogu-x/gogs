package model

import (
	"fmt"
	"log"
	"net"
	"os"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type inboundMsg struct {
	msg    proto.Message
	uid    uint64
	connID uint64
	gateId string
}

type GateActor struct {
	grpcServer *grpc.Server
	gamePID    actor.PID
	stream     protoGateway.Gateway_StreamServer
	router     actor.Router
}

func (g *GateActor) OnInit(ctx actor.ActorContext) {
	g.gamePID = actor.MustLookup(constant.ActorNats)

	g.router.Register(&inboundMsg{}, func(ctx actor.ActorContext, msg interface{}) {
		ctx.Send(g.gamePID, msg)
	})
	g.router.Register(&protoGateway.Frame{}, func(_ actor.ActorContext, msg interface{}) {
		if g.stream != nil {
			if err := g.stream.Send(msg.(*protoGateway.Frame)); err != nil {
				log.Printf("GateActor: stream send error: %v", err)
			}
		}
	})

	lis, err := net.Listen("tcp", config.GameAddr())
	if err != nil {
		log.Fatalf("GateActor: listen error: %v", err)
	}

	g.grpcServer = grpc.NewServer()
	protoGateway.RegisterGatewayServer(g.grpcServer, &gatewayService{actor: g, codec: codec.JsonCodec})

	go func() {
		log.Printf("GateActor: gRPC server listening on %s", config.GameAddr())
		if err := g.grpcServer.Serve(lis); err != nil {
			log.Printf("GateActor: grpc serve error: %v", err)
		}
	}()

	if err := cluster.Register(fmt.Sprintf("%d", config.ServerID), fmt.Sprintf("%d", os.Getpid()), config.GameAddr()); err != nil {
		log.Printf("GateActor: cluster register error: %v", err)
	} else {
		log.Printf("GateActor: registered [%d] -> %s", config.ServerID, config.GameAddr())
	}
}

func (g *GateActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GateActor) OnStop(ctx actor.ActorContext) {
	if g.grpcServer != nil {
		g.grpcServer.GracefulStop()
	}
}

type gatewayService struct {
	protoGateway.UnimplementedGatewayServer
	actor *GateActor
	codec codec.Codec
}

func (s *gatewayService) Stream(stream protoGateway.Gateway_StreamServer) error {
	s.actor.stream = stream
	self := actor.MustLookup(constant.ActorGate)

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
		actor.Send(self, &inboundMsg{
			msg:    protoMsg,
			uid:    frame.Uid,
			connID: frame.ConnId,
			gateId: frame.GateId,
		})
	}
}

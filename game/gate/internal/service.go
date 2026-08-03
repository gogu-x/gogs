package internal

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/codec"
	"github.com/gogu-x/gogs/config"
	"github.com/gogu-x/gogs/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"

	"google.golang.org/grpc"
)

type GateActor struct {
	grpcServer *grpc.Server
	playPID    tree.PID
	stream     protoGateway.Gateway_StreamServer
	router     tree.Router
}

func NewGateActor() *GateActor { return &GateActor{} }

func (g *GateActor) Name() string { return constant.Gate }

func (g *GateActor) OnInit(ctx tree.Context) {
	g.playPID = tree.MustLookup(constant.PLAY)

	g.router.Register(&protoGateway.Frame{}, func(_ tree.Context, msg interface{}) {
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

func (g *GateActor) HandleMessage(ctx tree.Context, msg interface{}) {
	g.router.Route(ctx, msg)
}

func (g *GateActor) OnStop(_ tree.Context) {
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
	playPID := tree.MustLookup(constant.PLAY)

	for {
		frame, err := stream.Recv()
		if err != nil {
			s.actor.stream = nil
			return err
		}
		// 入口统一为 Frame：解码与登录校验都由 Play 模块内部完成。
		tree.Send(playPID, frame)
	}
}

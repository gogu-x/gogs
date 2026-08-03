package stream

import (
	"context"
	"log"

	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"
	actor "github.com/gogu-x/tree"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Actor struct {
	serverID uint64
	stream   protoGateway.Gateway_StreamClient
	router   actor.Router
}

func New(serverID uint64) *Actor { return &Actor{serverID: serverID} }

func (s *Actor) Name() string { return constant.StreamName(s.serverID) }

func (s *Actor) OnInit(ctx actor.Context) {
	initRouter(s)

	addr, err := cluster.GetAddr(s.serverID)
	if err != nil {
		log.Printf("StreamActor[%s]: get addr error: %v", s.serverID, err)
		ctx.Stop()
		return
	}
	grpcConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("StreamActor[%s]: dial error: %v", s.serverID, err)
		ctx.Stop()
		return
	}
	stream, err := protoGateway.NewGatewayClient(grpcConn).Stream(context.Background())
	if err != nil {
		log.Printf("StreamActor[%s]: stream error: %v", s.serverID, err)
		ctx.Stop()
		return
	}
	s.stream = stream

	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				log.Printf("StreamActor[%s]: recv error: %v", s.serverID, err)
				return
			}
			if pid, ok := actor.Lookup(constant.ConnName(frame.ConnId)); ok {
				actor.Send(pid, frame)
			}
		}
	}()
}

func (s *Actor) HandleMessage(ctx actor.Context, msg interface{}) {
	s.router.Route(ctx, msg)
}

func (s *Actor) OnStop(_ actor.Context) {
	if s.stream != nil {
		_ = s.stream.CloseSend()
	}
}

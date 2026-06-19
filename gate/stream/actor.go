package stream

import (
	"context"
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/gate/constant"
	"github.com/gogu-x/gogs/pb/protoGateway"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type stopMsg struct{}

type Actor struct {
	serverID string
	stream   protoGateway.Gateway_StreamClient
	router   actor.Router
}

func New(serverID string) *Actor { return &Actor{serverID: serverID} }

func Name(serverID string) string { return fmt.Sprintf("stream-%s", serverID) }

func (s *Actor) OnInit(ctx actor.ActorContext) {
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

	self := ctx.Self()
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				log.Printf("StreamActor[%s]: recv error: %v", s.serverID, err)
				actor.Send(self, &stopMsg{})
				return
			}
			if pid, ok := actor.Lookup(constant.ConnName(frame.ConnId)); ok {
				actor.Send(pid, frame)
			}
		}
	}()
}

func (s *Actor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	s.router.Route(ctx, msg)
}

func (s *Actor) OnStop(_ actor.ActorContext) {
	if s.stream != nil {
		_ = s.stream.CloseSend()
	}
}

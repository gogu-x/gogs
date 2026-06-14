package server

import (
	"context"
	"fmt"
	"log"

	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/pb/protoGateway"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StreamActor struct {
	serverID string
	stream   protoGateway.Gateway_StreamClient
}

func NewStreamActor(serverID string) *StreamActor {
	return &StreamActor{serverID: serverID}
}

func StreamActorName(serverID string) string {
	return fmt.Sprintf("stream-%s", serverID)
}

func (s *StreamActor) OnInit(ctx actor.ActorContext) {
	addr, err := cluster.GetAddr(s.serverID)
	if err != nil {
		log.Printf("StreamActor[%s]: get game addr error: %v", s.serverID, err)
		ctx.Stop()
		return
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("StreamActor[%s]: grpc dial error: %v", s.serverID, err)
		ctx.Stop()
		return
	}

	stream, err := protoGateway.NewGatewayClient(conn).Stream(context.Background())
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
			connPID, ok := actor.Lookup(connActorName(frame.ConnId))
			if !ok {
				continue
			}
			actor.Send(connPID, frame)
		}
	}()
}

func (s *StreamActor) HandleMessage(ctx actor.ActorContext, msg interface{}) {
	switch m := msg.(type) {
	case *StreamMsg:
		if s.stream != nil {
			if err := s.stream.Send(m.Frame); err != nil {
				log.Printf("StreamActor[%s]: send error: %v", s.serverID, err)
				ctx.Stop()
			}
		}
	case *stopMsg:
		ctx.Stop()
	}
}

func (s *StreamActor) OnStop(ctx actor.ActorContext) {
	if s.stream != nil {
		_ = s.stream.CloseSend()
	}
}

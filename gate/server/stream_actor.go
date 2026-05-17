package server

import (
	"context"
	"fmt"
	actor "github.com/gogu-x/bigTree"
	"github.com/gogu-x/gogs/cluster"
	"github.com/gogu-x/gogs/pb/gateway"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StreamActor struct {
	serverID string
	stream   gateway.Gateway_StreamClient
}

type StreamMsg struct {
	Frame *gateway.Frame
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

	stream, err := gateway.NewGatewayClient(conn).Stream(context.Background())
	if err != nil {
		log.Printf("StreamActor[%s]: stream error: %v", s.serverID, err)
		ctx.Stop()
		return
	}
	s.stream = stream

	self := ctx.Self()
	sys := ctx.System()
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				log.Printf("StreamActor[%s]: recv error: %v", s.serverID, err)
				sys.Send(self, &stopMsg{})
				return
			}
			connPID, ok := sys.Lookup(connActorName(frame.ConnId))
			if !ok {
				continue
			}
			sys.Send(connPID, frame)
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

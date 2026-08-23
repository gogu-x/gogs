package internal

import (
	"errors"
	"io"
	"time"

	"github.com/gogu-x/gogs/pb/protoGateway"
	"github.com/gogu-x/tree"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type gatewayService struct {
	protoGateway.UnimplementedGatewayServer
	actorPID tree.PID
	system   *tree.Tree
}

// Stream owns gRPC reads for one Gate connection. All stateful processing and
// all writes are delegated to the matching ConnAgent Actor.
func (s *gatewayService) Stream(stream protoGateway.Gateway_StreamServer) error {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	result, err := s.system.Request(s.actorPID, &openSession{stream: stream, first: first}).AwaitTimeout(5 * time.Second)
	if err != nil {
		return status.Errorf(codes.Unavailable, "open gateway session: %v", err)
	}
	opened, ok := result.(*openedSession)
	if !ok {
		return status.Error(codes.Internal, "invalid gateway session response")
	}

	//客户端自动断开链接，或者下线
	defer s.system.Send(s.actorPID, &closeSession{
		key:        opened.key,
		pid:        opened.pid,
		generation: opened.generation,
		reason:     "stream closed",
	})

	for {
		frame, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := validateFrame(frame); err != nil {
			return status.Error(codes.PermissionDenied, err.Error())
		}
		if !s.system.TrySend(opened.pid, &inboundFrame{frame: frame}) {
			return status.Error(codes.ResourceExhausted, "gateway connection is overloaded")
		}
	}
}

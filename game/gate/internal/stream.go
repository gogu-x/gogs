package internal

import (
	"errors"
	"io"
	"time"

	"github.com/gogu-x/gogs/pb/cspb/pb_gateway"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/codec"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type gatewayService struct {
	pb_gateway.UnimplementedGatewayServer
	actorPID tree.PID
	system   *tree.Tree
	codec    codec.Codec
}

// Stream uses the first LoginGameReq payload to bind this stream to one UID.
func (s *gatewayService) Stream(stream pb_gateway.Gateway_StreamServer) error {
	frame, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	//解码消息
	msg, err := s.codec.Unmarshal(frame.Payload)
	if err != nil {
		return err
	}
	tlog.Log.Info("gateStream GateActor: receive msg: %s", msg)

	result, err := s.system.Request(
		s.actorPID,
		&openSession{stream: stream, msg: msg},
	).AwaitTimeout(5 * time.Second)
	if err != nil {
		return status.Errorf(codes.PermissionDenied, "open gateway session: %v", err)
	}
	opened, ok := result.(*openedSession)
	if !ok {
		return status.Error(codes.Internal, "invalid gateway session response")
	}

	defer s.system.Send(s.actorPID, &closeSession{
		pid:        opened.pid,
		uid:        opened.uid,
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
			return status.Error(codes.InvalidArgument, err.Error())
		}
		//解码消息
		msg, err := s.codec.Unmarshal(frame.Payload)
		if err != nil {
			return err
		}
		tlog.Log.Info("gateStream GateActor: receive msg: %v", msg)
		if !s.system.TrySend(opened.pid, &inboundFrame{msg: msg}) {
			return status.Error(codes.ResourceExhausted, "gateway connection is overloaded")
		}
	}
}

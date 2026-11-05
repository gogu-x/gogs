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

	// Decode the first session frame before opening an actor session.
	msg, err := s.codec.Unmarshal(frame.Payload)
	if err != nil {
		tlog.Log.Error("[gate/gatewayService.Stream] 初始网关消息解码失败, err=%v", err)
		return status.Error(codes.InvalidArgument, "")
	}
	tlog.Log.Debug("[gate/gatewayService.Stream] 收到初始网关消息, type=%T", msg)

	result, err := s.system.Request(
		s.actorPID,
		&openSession{stream: stream, msg: msg},
	).AwaitTimeout(5 * time.Second)

	if err != nil {
		tlog.Log.Error("[gate/gatewayService.Stream] 创建网关会话失败, err=%v", err)
		return status.Error(codes.PermissionDenied, "")
	}
	opened, ok := result.(*openedSession)
	if !ok {
		tlog.Log.Error("[gate/gatewayService.Stream] 网关会话响应类型无效, type=%T", result)
		return status.Error(codes.Internal, "")
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
			tlog.Log.Error("[gate/gatewayService.Stream] 网关消息校验失败, err=%v pid=%v", err, opened.uid)
			return status.Error(codes.InvalidArgument, "")
		}
		// Decode only validated transport frames.
		msg, err := s.codec.Unmarshal(frame.Payload)
		if err != nil {
			tlog.Log.Error("[gate/gatewayService.Stream] 网关消息解码失败, err=%v pid=%v", err, opened.uid)
			return status.Error(codes.InvalidArgument, "")
		}
		tlog.Log.Debug("[gate/gatewayService.Stream] 收到网关消息, type=%T", msg)
		if !s.system.TrySend(opened.pid, &inboundFrame{msg: msg}) {
			tlog.Log.Error("[gate/gatewayService.Stream] 网关会话队列已满, pid=%v", opened.uid)
			return status.Error(codes.ResourceExhausted, "")
		}
	}
}
